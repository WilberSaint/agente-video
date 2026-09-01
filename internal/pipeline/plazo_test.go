package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"agente-video/internal/perfil"
	"agente-video/internal/proveedor"
)

// guionistaLento simula lo que se vio en producción: una etapa que se queda
// esperando y no vuelve. Antes del plazo esto bloqueaba la cola entera.
type guionistaLento struct{ arrancado chan struct{} }

func (g *guionistaLento) Nombre() string { return "lento" }

func (g *guionistaLento) Generar(ctx context.Context, _ *perfil.Perfil, _ string) (*proveedor.GuionGenerado, error) {
	close(g.arrancado)
	<-ctx.Done() // nunca termina por su cuenta
	return nil, ctx.Err()
}

func TestElPlazoCortaUnaEtapaDetenida(t *testing.T) {
	pl := Nuevo(Proveedores{Guionista: &guionistaLento{arrancado: make(chan struct{})}}, Opciones{
		DirTrabajo:  t.TempDir(),
		DirSalida:   t.TempDir(),
		PlazoMaximo: 300 * time.Millisecond,
		Registro:    func(string, ...any) {},
	})

	hecho := make(chan error, 1)
	go func() {
		_, err := pl.Ejecutar(context.Background(), &perfil.Perfil{ID: "x"}, "un tema", "")
		hecho <- err
	}()

	select {
	case err := <-hecho:
		if err == nil {
			t.Fatal("se esperaba un error al agotarse el plazo")
		}
		// El mensaje tiene que decir que fue el plazo: si llega como
		// "context deadline exceeded" nadie sabe qué pasó a las 3 de la mañana.
		if !strings.Contains(err.Error(), "se agotó el plazo") {
			t.Errorf("el error no explica la causa: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el plazo no cortó nada: el trabajo sigue colgado")
	}
}

func TestSinPlazoExplicitoSePoneElPorDefecto(t *testing.T) {
	pl := Nuevo(Proveedores{}, Opciones{})
	if pl.opt.PlazoMaximo != plazoPorDefecto {
		t.Errorf("PlazoMaximo = %v, se esperaba %v", pl.opt.PlazoMaximo, plazoPorDefecto)
	}
}

// Pollinations solo admite una petición por IP a la vez. Reintentar un 429 a
// los tres segundos vuelve a chocar con la misma cola y agota los intentos sin
// haber esperado de verdad: fue lo que tumbó dos videos seguidos.
func TestEsperaDeReintentoDaMasMargenAlSaturarse(t *testing.T) {
	saturado := errors.New(`pollinations devolvió 429: {"error":"Too Many Requests"}`)
	otro := errors.New("connection reset by peer")

	for intento := 1; intento <= 3; intento++ {
		conCola := esperaDeReintento(saturado, intento)
		normal := esperaDeReintento(otro, intento)
		if conCola <= normal {
			t.Errorf("intento %d: 429 esperó %v y un error normal %v; debería esperar más",
				intento, conCola, normal)
		}
	}
	// Los tres intentos juntos tienen que cubrir de sobra el tiempo que tarda
	// otro video en soltar la cola.
	var total time.Duration
	for intento := 1; intento <= 3; intento++ {
		total += esperaDeReintento(saturado, intento)
	}
	if total < time.Minute {
		t.Errorf("los tres reintentos suman %v; es poco para que se libere la cola", total)
	}
}

func TestDormirVuelveAlCancelar(t *testing.T) {
	pl := Nuevo(Proveedores{}, Opciones{Registro: func(string, ...any) {}})
	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()

	hecho := make(chan struct{})
	go func() { pl.dormir(ctx, time.Hour); close(hecho) }()
	select {
	case <-hecho:
	case <-time.After(2 * time.Second):
		t.Fatal("dormir ignoró la cancelación; parar un trabajo tardaría una hora")
	}
}
