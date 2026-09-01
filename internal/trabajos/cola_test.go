package trabajos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agente-video/internal/pipeline"
)

// esperar sondea hasta que se cumpla la condición o se agote el plazo. Los
// cambios de estado ocurren en otra goroutine, así que comprobarlos justo
// después de encolar sería una carrera.
func esperar(t *testing.T, plazo time.Duration, que func() bool) {
	t.Helper()
	limite := time.Now().Add(plazo)
	for time.Now().Before(limite) {
		if que() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("se agotó el plazo de %s esperando la condición", plazo)
}

func colaDePrueba(t *testing.T, ej Ejecutor) *Cola {
	t.Helper()
	c := NuevaCola(filepath.Join(t.TempDir(), "cola.json"), ej)
	ctx, cancelar := context.WithCancel(context.Background())
	c.Arrancar(ctx)
	t.Cleanup(func() {
		cancelar()
		// Tope generoso: si el obrero está sano sale enseguida, y bajo carga
		// —estas pruebas pueden correr mientras se renderiza un video— tres
		// segundos no bastaban y el TempDir se borraba a media escritura.
		c.Esperar(20 * time.Second)
	})
	return c
}

func TestUnTrabajoLlegaATerminado(t *testing.T) {
	c := colaDePrueba(t, func(ctx context.Context, tr *Trabajo,
		av func(pipeline.Avance), reg func(string)) (*Resultado, error) {
		av(pipeline.Avance{Etapa: 2, Etiqueta: "imágenes", Hecho: 1, Total: 2})
		reg("una línea")
		return &Resultado{Titulo: "Título", Video: "v.mp4"}, nil
	})

	tr, err := c.Encolar("demo", "un tema")
	if err != nil {
		t.Fatal(err)
	}
	esperar(t, 20*time.Second, func() bool { return c.Uno(tr.ID).Estado == Terminado })

	final := c.Uno(tr.ID)
	if final.Titulo != "Título" {
		t.Errorf("título = %q", final.Titulo)
	}
	if final.Progreso != 1 {
		t.Errorf("progreso = %v, esperaba 1 al terminar", final.Progreso)
	}
	if len(final.Registro) == 0 {
		t.Error("no se guardó el registro")
	}
	if final.Fin == nil {
		t.Error("falta la marca de fin")
	}
}

func TestUnErrorDejaElTrabajoFallido(t *testing.T) {
	c := colaDePrueba(t, func(ctx context.Context, tr *Trabajo,
		av func(pipeline.Avance), reg func(string)) (*Resultado, error) {
		return nil, fmt.Errorf("se rompió algo")
	})

	tr, _ := c.Encolar("demo", "tema")
	esperar(t, 20*time.Second, func() bool { return c.Uno(tr.ID).Estado == Fallido })

	if got := c.Uno(tr.ID).Error; got != "se rompió algo" {
		t.Errorf("error = %q", got)
	}
}

// Se procesan de uno en uno: el montaje satura los cuatro núcleos y dos a la
// vez no irían al doble de velocidad.
func TestSoloUnTrabajoALaVez(t *testing.T) {
	simultaneos := make(chan struct{}, 8)
	soltar := make(chan struct{})

	c := colaDePrueba(t, func(ctx context.Context, tr *Trabajo,
		av func(pipeline.Avance), reg func(string)) (*Resultado, error) {
		simultaneos <- struct{}{}
		<-soltar
		<-simultaneos
		return &Resultado{}, nil
	})

	for i := 0; i < 3; i++ {
		if _, err := c.Encolar("demo", fmt.Sprintf("tema %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	esperar(t, 20*time.Second, func() bool { return len(simultaneos) == 1 })

	// Margen generoso: si el obrero arrancase otro indebidamente, aquí se vería.
	// Va holgado porque estas pruebas corren mientras la máquina puede estar
	// renderizando un video y saturando los cuatro núcleos.
	time.Sleep(400 * time.Millisecond)
	if n := len(simultaneos); n != 1 {
		t.Fatalf("%d trabajos a la vez, esperaba 1", n)
	}
	close(soltar)
}

// bloqueante devuelve un ejecutor que avisa por canal cuando arranca y se queda
// esperando. Sincronizar así, en vez de sondear el estado con un reloj, hace la
// prueba independiente de lo cargada que esté la máquina: estas corren mientras
// el servidor puede estar renderizando un video con los cuatro núcleos al 100%.
func bloqueante(arrancado chan<- string, soltar <-chan struct{}) Ejecutor {
	return func(ctx context.Context, tr *Trabajo,
		av func(pipeline.Avance), reg func(string)) (*Resultado, error) {
		arrancado <- tr.ID
		<-soltar
		return &Resultado{}, nil
	}
}

func TestCancelarUnoEnCola(t *testing.T) {
	arrancado := make(chan string, 4)
	soltar := make(chan struct{})
	defer close(soltar)

	c := colaDePrueba(t, bloqueante(arrancado, soltar))

	primero, _ := c.Encolar("demo", "el que corre")
	segundo, _ := c.Encolar("demo", "el que espera")

	// Cuando el ejecutor arranca, el estado ya se marcó como Corriendo.
	select {
	case id := <-arrancado:
		if id != primero.ID {
			t.Fatalf("arrancó %s primero; el orden de la cola no se respetó", id)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("el primer trabajo nunca arrancó")
	}

	if err := c.Cancelar(segundo.ID); err != nil {
		t.Fatal(err)
	}
	if got := c.Uno(segundo.ID).Estado; got != Cancelado {
		t.Errorf("estado = %q, esperaba cancelado", got)
	}
	if got := c.Uno(primero.ID).Estado; got != Corriendo {
		t.Errorf("cancelar el segundo afectó al primero: %q", got)
	}
}

func TestNoSePuedeOlvidarUnoEnCurso(t *testing.T) {
	arrancado := make(chan string, 2)
	soltar := make(chan struct{})
	defer close(soltar)

	c := colaDePrueba(t, bloqueante(arrancado, soltar))
	tr, _ := c.Encolar("demo", "tema")

	select {
	case <-arrancado:
	case <-time.After(20 * time.Second):
		t.Fatal("el trabajo nunca arrancó")
	}

	if err := c.Olvidar(tr.ID); err == nil {
		t.Error("dejó quitar un trabajo en curso; debería exigir cancelarlo antes")
	}
}

// Lo que motiva el campo Carpeta: tras un reinicio el trabajo debe retomar sus
// checkpoints, no volver a generar las imágenes ya pagadas.
func TestUnReinicioConservaLaCarpeta(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "cola.json")

	previos := []*Trabajo{{
		ID: "abc", Perfil: "demo", Tema: "tema", Estado: Corriendo,
		Carpeta: "20260101-120000-tema", Creado: time.Now(),
	}}
	datos, _ := json.Marshal(previos)
	if err := os.WriteFile(ruta, datos, 0o644); err != nil {
		t.Fatal(err)
	}

	visto := make(chan string, 1)
	c := NuevaCola(ruta, func(ctx context.Context, tr *Trabajo,
		av func(pipeline.Avance), reg func(string)) (*Resultado, error) {
		visto <- tr.Carpeta
		return &Resultado{}, nil
	})

	// Al cargar, un "corriendo" huérfano vuelve a la cola: nadie lo está haciendo.
	if got := c.Uno("abc").Estado; got != EnCola {
		t.Fatalf("estado tras cargar = %q, esperaba en_cola", got)
	}

	ctx, cancelar := context.WithCancel(context.Background())
	c.Arrancar(ctx)
	// Cancelar y esperar en el mismo defer, y en ese orden: el obrero escribe
	// cola.json al cerrar cada trabajo, y si el test termina antes lo pilla la
	// limpieza del TempDir a media escritura.
	defer func() {
		cancelar()
		c.Esperar(20 * time.Second)
	}()

	select {
	case carpeta := <-visto:
		if carpeta != "20260101-120000-tema" {
			t.Errorf("carpeta = %q; se perdieron los checkpoints", carpeta)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("el trabajo reencolado nunca arrancó")
	}
}

func TestEncolarValidaLaEntrada(t *testing.T) {
	c := NuevaCola("", nil)
	if _, err := c.Encolar("demo", ""); err == nil {
		t.Error("aceptó un tema vacío")
	}
	if _, err := c.Encolar("", "tema"); err == nil {
		t.Error("aceptó un perfil vacío")
	}
}

func TestSanear(t *testing.T) {
	casos := map[string]string{
		"El faro que nadie visita": "el-faro-que-nadie-visita",
		"¿Por qué?  ¡Sí!":          "por-que-si",
		"":                         "video",
		"---":                      "video",
	}
	for entrada, quiero := range casos {
		if got := sanear(entrada, 40); got != quiero {
			t.Errorf("sanear(%q) = %q, esperaba %q", entrada, got, quiero)
		}
	}
}

func TestFraccionPonderaLasEtapas(t *testing.T) {
	// La etapa de imágenes se lleva la mayor parte del tiempo real, así que
	// terminarla debe dejar la barra bastante avanzada, no al 40%.
	trasImagenes := pipeline.Avance{Etapa: 2, Hecho: 1, Total: 1}.Fraccion()
	if trasImagenes < 0.6 {
		t.Errorf("acabar las imágenes deja la barra en %.2f, esperaba más de 0.6", trasImagenes)
	}
	if f := (pipeline.Avance{Etapa: 1, Hecho: 0, Total: 1}).Fraccion(); f != 0 {
		t.Errorf("empezar debería ser 0, fue %.2f", f)
	}
	if f := (pipeline.Avance{Etapa: 5, Hecho: 1, Total: 1}).Fraccion(); f < 0.99 {
		t.Errorf("acabar el montaje debería ser ~1, fue %.2f", f)
	}
}

// Encolar varios temas de golpe es la función principal del panel, y los
// identificadores se generaban desde el reloj. En Windows su resolución es de
// medio milisegundo: al encolar en bucle todos recibían el mismo instante y el
// mismo id, con lo que el mapa se quedaba con uno y cancelar un trabajo
// afectaba a otro. Medido antes del arreglo: 2 ids distintos de 8.
func TestLosIdentificadoresNoColisionan(t *testing.T) {
	c := NuevaCola("", nil) // sin obrero: solo interesa el alta

	const cuantos = 200
	vistos := make(map[string]string, cuantos)
	for i := 0; i < cuantos; i++ {
		tr, err := c.Encolar("demo", fmt.Sprintf("tema %d", i))
		if err != nil {
			t.Fatal(err)
		}
		if previo, repetido := vistos[tr.ID]; repetido {
			t.Fatalf("id repetido %q entre %q y %q", tr.ID, previo, tr.Tema)
		}
		vistos[tr.ID] = tr.Tema
	}
	if len(c.Listar()) != cuantos {
		t.Errorf("la cola tiene %d trabajos, esperaba %d: se perdieron por id repetido",
			len(c.Listar()), cuantos)
	}
}

func TestAtascoCuentaSoloLoQueSigueVivo(t *testing.T) {
	c := NuevaCola(filepath.Join(t.TempDir(), "cola.json"), nil)
	if n := c.Atasco(); n != 0 {
		t.Fatalf("cola vacía: Atasco() = %d", n)
	}
	for i := 0; i < 3; i++ {
		if _, err := c.Encolar("p", fmt.Sprintf("tema %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if n := c.Atasco(); n != 3 {
		t.Fatalf("Atasco() = %d, se esperaban 3", n)
	}
	// Un trabajo cerrado ya no atasca a nadie, aunque siga en la lista.
	c.mu.Lock()
	c.trabajos[0].Estado = Terminado
	c.trabajos[1].Estado = Fallido
	c.mu.Unlock()
	if n := c.Atasco(); n != 1 {
		t.Errorf("Atasco() = %d tras cerrar dos, se esperaba 1", n)
	}
}
