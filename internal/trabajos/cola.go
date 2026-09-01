// Package trabajos gestiona la cola de generación.
//
// Un solo obrero procesa los trabajos de uno en uno, a propósito. El montaje
// satura los cuatro núcleos del servidor, así que dos videos a la vez no van al
// doble de velocidad: van los dos a la mitad, y con el disco a 26 MB/s peor.
// En serie, además, el progreso que ve la persona significa algo.
package trabajos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"agente-video/internal/pipeline"
)

type Estado string

const (
	EnCola    Estado = "en_cola"
	Corriendo Estado = "corriendo"
	Terminado Estado = "terminado"
	Fallido   Estado = "fallido"
	Cancelado Estado = "cancelado"
)

type Trabajo struct {
	ID     string `json:"id"`
	Perfil string `json:"perfil"`
	Tema   string `json:"tema"`
	Estado Estado `json:"estado"`

	Etapa    string  `json:"etapa"`
	Detalle  string  `json:"detalle"`
	Progreso float64 `json:"progreso"` // 0..1

	Creado time.Time  `json:"creado"`
	Inicio *time.Time `json:"inicio,omitempty"`
	Fin    *time.Time `json:"fin,omitempty"`

	Error     string `json:"error,omitempty"`
	Titulo    string `json:"titulo,omitempty"`
	Video     string `json:"video,omitempty"`     // ruta en disco
	Miniatura string `json:"miniatura,omitempty"` // ruta en disco
	Textos    string `json:"textos,omitempty"`

	// Carpeta es el identificador del directorio de checkpoints. Se fija en el
	// primer intento y se conserva: sin esto, un trabajo reencolado tras un
	// reinicio empezaría con carpeta nueva y volvería a generar imágenes que ya
	// están en disco, tirando a la basura los minutos —y el dinero— gastados.
	Carpeta string `json:"carpeta,omitempty"`

	// Registro guarda las últimas líneas para poder mirar qué pasó sin ir a la
	// terminal. Se acota: un trabajo largo genera cientos y no aportan.
	Registro []string `json:"registro,omitempty"`
}

const maxRegistro = 60

// Resultado es lo que devuelve el ejecutor cuando un trabajo sale bien.
type Resultado struct {
	Titulo string
	Video  string
	Textos string
}

// Ejecutor hace el trabajo de verdad. Se inyecta para que este paquete no
// sepa nada de perfiles, proveedores ni ffmpeg.
type Ejecutor func(ctx context.Context, t *Trabajo,
	avance func(pipeline.Avance), registrar func(string)) (*Resultado, error)

type Evento struct {
	Tipo    string   `json:"tipo"` // "estado" | "lista"
	Trabajo *Trabajo `json:"trabajo,omitempty"`
}

type Cola struct {
	mu       sync.RWMutex
	trabajos []*Trabajo
	porID    map[string]*Trabajo

	ejecutor  Ejecutor
	ruta      string // dónde se persiste
	cancelar  context.CancelFunc
	corriendo string // id del trabajo en curso

	suscriptores map[chan Evento]struct{}
	despertar    chan struct{}
	hecho        chan struct{}
}

func NuevaCola(ruta string, ejecutor Ejecutor) *Cola {
	c := &Cola{
		porID:        map[string]*Trabajo{},
		ejecutor:     ejecutor,
		ruta:         ruta,
		suscriptores: map[chan Evento]struct{}{},
		despertar:    make(chan struct{}, 1),
	}
	c.cargar()
	return c
}

// Arrancar lanza el obrero. Vuelve enseguida; el trabajo ocurre en segundo plano.
func (c *Cola) Arrancar(ctx context.Context) {
	c.hecho = make(chan struct{})
	go c.obrero(ctx)
}

// Esperar bloquea hasta que el obrero termina de recoger. Importa porque el
// obrero escribe cola.json al cerrar cada trabajo: cortarlo a media escritura
// dejaría el archivo corrupto y se perdería la cola entera al reiniciar.
func (c *Cola) Esperar(plazo time.Duration) {
	if c.hecho == nil {
		return
	}
	select {
	case <-c.hecho:
	case <-time.After(plazo):
	}
}

func (c *Cola) obrero(ctx context.Context) {
	defer close(c.hecho)
	for {
		t := c.siguiente()
		if t == nil {
			select {
			case <-ctx.Done():
				return
			case <-c.despertar:
				continue
			case <-time.After(2 * time.Second):
				continue
			}
		}
		c.correr(ctx, t)
	}
}

func (c *Cola) siguiente() *Trabajo {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.trabajos {
		if t.Estado == EnCola {
			return t
		}
	}
	return nil
}

func (c *Cola) correr(padre context.Context, t *Trabajo) {
	ctx, cancelar := context.WithCancel(padre)

	ahora := time.Now()
	c.mu.Lock()
	if t.Carpeta == "" {
		// Se fija una sola vez, con la fecha de creación y no la de arranque,
		// para que reintentar no cambie la carpeta ni pierda los checkpoints.
		t.Carpeta = fmt.Sprintf("%s-%s", t.Creado.Format("20060102-150405"), sanear(t.Tema, 40))
	}
	t.Estado = Corriendo
	t.Inicio = &ahora
	t.Etapa = "arrancando"
	t.Progreso = 0
	t.Error = ""
	t.Registro = nil
	c.cancelar = cancelar
	c.corriendo = t.ID
	c.mu.Unlock()

	c.guardar()
	c.emitir(Evento{Tipo: "estado", Trabajo: c.Uno(t.ID)})

	// El progreso se emite tal cual llega: la etapa de imágenes puede tardar
	// diez minutos y sin señal la interfaz parecería colgada.
	avance := func(a pipeline.Avance) {
		c.mu.Lock()
		t.Etapa = a.Etiqueta
		t.Detalle = a.Detalle
		t.Progreso = a.Fraccion()
		c.mu.Unlock()
		c.emitir(Evento{Tipo: "estado", Trabajo: c.Uno(t.ID)})
	}

	registrar := func(linea string) {
		c.mu.Lock()
		t.Registro = append(t.Registro, linea)
		if len(t.Registro) > maxRegistro {
			t.Registro = t.Registro[len(t.Registro)-maxRegistro:]
		}
		c.mu.Unlock()
	}

	res, err := c.ejecutor(ctx, t, avance, registrar)

	// Hay que mirar el contexto ANTES de cancelarlo: después siempre está
	// cancelado y todo fallo se confundiría con una cancelación manual.
	interrumpido := ctx.Err() != nil
	cancelar()

	fin := time.Now()
	c.mu.Lock()
	c.cancelar = nil
	c.corriendo = ""
	t.Fin = &fin
	switch {
	case interrumpido && err != nil:
		// Cancelado a mano: no es un fallo, y conviene distinguirlo para no
		// perseguir un error que no existe.
		t.Estado = Cancelado
		t.Etapa = "cancelado"
		t.Detalle = ""
	case err != nil:
		t.Estado = Fallido
		t.Etapa = "falló"
		t.Error = err.Error()
	default:
		t.Estado = Terminado
		t.Etapa = "listo"
		t.Detalle = ""
		t.Progreso = 1
		if res != nil {
			t.Titulo = res.Titulo
			t.Video = res.Video
			t.Textos = res.Textos
		}
	}
	copia := t.copia()
	c.mu.Unlock()

	c.guardar()
	c.emitir(Evento{Tipo: "estado", Trabajo: copia})
}

// Encolar añade un trabajo al final.
func (c *Cola) Encolar(perfil, tema string) (*Trabajo, error) {
	if tema == "" {
		return nil, fmt.Errorf("hace falta un tema")
	}
	if perfil == "" {
		return nil, fmt.Errorf("hace falta un perfil")
	}
	t := &Trabajo{
		ID:     fmt.Sprintf("%d-%s", time.Now().UnixNano(), aleatorio()),
		Perfil: perfil,
		Tema:   tema,
		Estado: EnCola,
		Creado: time.Now(),
		Etapa:  "en cola",
	}
	c.mu.Lock()
	c.trabajos = append(c.trabajos, t)
	c.porID[t.ID] = t
	c.mu.Unlock()

	c.guardar()
	c.emitir(Evento{Tipo: "estado", Trabajo: t.copia()})
	select {
	case c.despertar <- struct{}{}:
	default:
	}
	return t, nil
}

// Cancelar quita de la cola, o aborta el trabajo en curso.
func (c *Cola) Cancelar(id string) error {
	c.mu.Lock()
	t, ok := c.porID[id]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("no existe el trabajo %s", id)
	}
	switch t.Estado {
	case EnCola:
		t.Estado = Cancelado
		t.Etapa = "cancelado antes de empezar"
	case Corriendo:
		if c.cancelar != nil {
			c.cancelar()
		}
	default:
		c.mu.Unlock()
		return fmt.Errorf("el trabajo ya terminó")
	}
	copia := t.copia()
	c.mu.Unlock()

	c.guardar()
	c.emitir(Evento{Tipo: "estado", Trabajo: copia})
	return nil
}

// Olvidar borra un trabajo de la lista. No toca los archivos generados.
func (c *Cola) Olvidar(id string) error {
	c.mu.Lock()
	t, ok := c.porID[id]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("no existe el trabajo %s", id)
	}
	if t.Estado == Corriendo {
		c.mu.Unlock()
		return fmt.Errorf("cancélalo antes de quitarlo")
	}
	delete(c.porID, id)
	for i, x := range c.trabajos {
		if x.ID == id {
			c.trabajos = append(c.trabajos[:i], c.trabajos[i+1:]...)
			break
		}
	}
	c.mu.Unlock()

	c.guardar()
	c.emitir(Evento{Tipo: "lista"})
	return nil
}

// Listar devuelve copias, ordenadas: primero lo activo, luego lo terminado por
// fecha descendente. Copias porque el obrero muta los originales.
func (c *Cola) Listar() []*Trabajo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]*Trabajo, 0, len(c.trabajos))
	for _, t := range c.trabajos {
		out = append(out, t.copia())
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := prioridad(out[i].Estado), prioridad(out[j].Estado)
		if pi != pj {
			return pi < pj
		}
		return out[i].Creado.After(out[j].Creado)
	})
	return out
}

func (c *Cola) Uno(id string) *Trabajo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if t, ok := c.porID[id]; ok {
		return t.copia()
	}
	return nil
}

func prioridad(e Estado) int {
	switch e {
	case Corriendo:
		return 0
	case EnCola:
		return 1
	default:
		return 2
	}
}

// --- suscripción a eventos (para SSE) ---

func (c *Cola) Suscribir() (<-chan Evento, func()) {
	ch := make(chan Evento, 16)
	c.mu.Lock()
	c.suscriptores[ch] = struct{}{}
	c.mu.Unlock()

	return ch, func() {
		c.mu.Lock()
		delete(c.suscriptores, ch)
		close(ch)
		c.mu.Unlock()
	}
}

func (c *Cola) emitir(e Evento) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for ch := range c.suscriptores {
		// Sin bloquear: un cliente lento no puede frenar la generación.
		select {
		case ch <- e:
		default:
		}
	}
}

// --- persistencia ---

func (c *Cola) guardar() {
	if c.ruta == "" {
		return
	}
	c.mu.RLock()
	datos, err := json.MarshalIndent(c.trabajos, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(c.ruta), 0o755)
	// Si la carpeta ya no existe, no se recrea. Sin esta comprobación, un
	// guardado tardío del obrero podía resucitar el directorio justo cuando
	// alguien acababa de borrarlo, y dejarlo con un archivo suelto dentro.
	if _, err := os.Stat(filepath.Dir(c.ruta)); err != nil {
		return
	}

	// Escritura atómica: un corte a media escritura dejaría cola.json corrupto
	// y se perdería la cola entera. Si el rename falla, el temporal se limpia
	// para no dejar basura en la carpeta.
	tmp := c.ruta + ".tmp"
	if os.WriteFile(tmp, datos, 0o644) != nil {
		return
	}
	if os.Rename(tmp, c.ruta) != nil {
		_ = os.Remove(tmp)
	}
}

func (c *Cola) cargar() {
	datos, err := os.ReadFile(c.ruta)
	if err != nil {
		return
	}
	var previos []*Trabajo
	if json.Unmarshal(datos, &previos) != nil {
		return
	}
	for _, t := range previos {
		// Si el servidor murió a mitad de un trabajo, al volver no está
		// corriendo nadie: se devuelve a la cola en lugar de dejarlo mintiendo.
		if t.Estado == Corriendo {
			t.Estado = EnCola
			t.Etapa = "reencolado tras reinicio"
			t.Progreso = 0
		}
		c.trabajos = append(c.trabajos, t)
		c.porID[t.ID] = t
	}
}

func (t *Trabajo) copia() *Trabajo {
	c := *t
	c.Registro = append([]string(nil), t.Registro...)
	return &c
}

func aleatorio() string {
	const letras = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 5)
	n := time.Now().UnixNano()
	for i := range b {
		b[i] = letras[n%int64(len(letras))]
		n /= int64(len(letras))
	}
	return string(b)
}

// sanear convierte un tema en un nombre de carpeta legible. Se duplica aquí en
// vez de exportarla desde pipeline para no ampliar su superficie pública por un
// detalle de nombres de archivo.
func sanear(s string, max int) string {
	reemplazos := map[rune]rune{'á': 'a', 'é': 'e', 'í': 'i', 'ó': 'o', 'ú': 'u', 'ñ': 'n', 'ü': 'u'}
	var b []rune
	guion := false
	for _, r := range s {
		if v, ok := reemplazos[r]; ok {
			r = v
		}
		if r >= 'A' && r <= 'Z' {
			r += 32
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b = append(b, r)
			guion = false
			continue
		}
		if !guion && len(b) > 0 {
			b = append(b, '-')
			guion = true
		}
	}
	for len(b) > 0 && b[len(b)-1] == '-' {
		b = b[:len(b)-1]
	}
	if len(b) > max {
		b = b[:max]
		for len(b) > 0 && b[len(b)-1] == '-' {
			b = b[:len(b)-1]
		}
	}
	if len(b) == 0 {
		return "video"
	}
	return string(b)
}
