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
