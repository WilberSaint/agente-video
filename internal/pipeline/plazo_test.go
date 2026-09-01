package pipeline

import (
	"context"
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
