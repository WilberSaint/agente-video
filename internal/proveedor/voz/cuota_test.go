package voz

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"agente-video/internal/proveedor"
)

// locutorQueFalla simula un proveedor de pago que rechaza la petición por un
// motivo que no cambia reintentando.
type locutorQueFalla struct{ err error }

func (l *locutorQueFalla) Nombre() string { return "falla" }
func (l *locutorQueFalla) Sintetizar(context.Context, proveedor.PeticionVoz) error {
	return l.err
}

type locutorQueAnota struct{ llamado bool }

func (l *locutorQueAnota) Nombre() string { return "suplente" }
func (l *locutorQueAnota) Sintetizar(context.Context, proveedor.PeticionVoz) error {
	l.llamado = true
	return nil
}

// El caso real: ElevenLabs devolvió 402 porque el plan gratuito no admite voces
// de la biblioteca, y el video entero falló en vez de salir con la voz de
// respaldo. De madrugada eso es una noche perdida por una voz.
func TestRespaldoEntraConUnErrorPermanente(t *testing.T) {
	for _, c := range []struct {
		nombre string
		err    error
	}{
		{"402 del plan gratuito", proveedor.Permanente(errors.New("elevenlabs devolvió 402"))},
		{"401 credencial", proveedor.Permanente(errors.New("elevenlabs devolvió 401"))},
		{"429 ritmo", errors.New("elevenlabs devolvió 429")},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			suplente := &locutorQueAnota{}
			cr := &ConRespaldo{
				Principal: &locutorQueFalla{err: c.err},
				Respaldo:  suplente,
				Contador:  NuevoContador(filepath.Join(t.TempDir(), "c.json")),
			}
			if err := cr.Sintetizar(context.Background(), proveedor.PeticionVoz{Texto: "hola"}); err != nil {
				t.Fatalf("debería haber salido adelante con el respaldo: %v", err)
			}
			if !suplente.llamado {
				t.Error("no se usó el respaldo")
			}
		})
	}
}

// Un fallo pasajero sí debe propagarse: cambiar de voz por un corte de red
// sería peor que reintentar.
func TestRespaldoNoEntraConUnFalloPasajero(t *testing.T) {
	suplente := &locutorQueAnota{}
	cr := &ConRespaldo{
		Principal: &locutorQueFalla{err: errors.New("conexión reiniciada")},
		Respaldo:  suplente,
		Contador:  NuevoContador(filepath.Join(t.TempDir(), "c.json")),
	}
	if err := cr.Sintetizar(context.Background(), proveedor.PeticionVoz{Texto: "hola"}); err == nil {
		t.Error("se esperaba que el error se propagara")
	}
	if suplente.llamado {
		t.Error("no debía usarse el respaldo por un fallo pasajero")
	}
}

func TestAcotarVelocidad(t *testing.T) {
	casos := []struct {
		dada   float64
		quiere float64
		avisa  bool
	}{
		{0, 1, false}, // sin configurar: velocidad natural
		{1.1, 1.1, false},
		{0.7, 0.7, false},
		{1.2, 1.2, false},
		{0.96, 0.96, false}, // el valor que ya traen los perfiles de Piper
		{1.5, 1.2, true},    // por encima: la API daría 422
		{0.3, 0.7, true},    // por debajo: igual
	}
	for _, c := range casos {
		avisado := false
		got := acotarVelocidad(c.dada, func(string, ...any) { avisado = true })
		if got != c.quiere {
			t.Errorf("acotarVelocidad(%v) = %v, se esperaba %v", c.dada, got, c.quiere)
		}
		if avisado != c.avisa {
			t.Errorf("acotarVelocidad(%v): avisó=%v, se esperaba %v", c.dada, avisado, c.avisa)
		}
	}
}

// Sin Aviso conectado no debe reventar: el aviso es opcional.
func TestAcotarVelocidadSinAviso(t *testing.T) {
	if got := acotarVelocidad(9, nil); got != velocidadMax {
		t.Errorf("= %v, se esperaba %v", got, velocidadMax)
	}
}
