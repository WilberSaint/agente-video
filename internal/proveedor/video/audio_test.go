package video

import (
	"testing"
	"time"

	"agente-video/internal/proveedor"
)

func tramosDe(n int, paso time.Duration) []tramo {
	ts := make([]tramo, n)
	for i := range ts {
		ts[i] = tramo{inicio: time.Duration(i) * paso, fin: time.Duration(i+1) * paso}
	}
	return ts
}

// En modo "escena" el efecto marca el cambio de idea, no cada imagen: dentro de
// una escena los planos siguen hablando de lo mismo y marcarlos todos cansa.
func TestInstantesModoEscena(t *testing.T) {
	escenas := []proveedor.EscenaRender{
		{Imagenes: []string{"a", "b"}}, // tramos 0 y 1
		{Imagenes: []string{"c"}},      // tramo 2
		{Imagenes: []string{"d", "e"}}, // tramos 3 y 4
	}
	ts := tramosDe(5, time.Second)
	got := instantesDeEfecto(escenas, ts, "escena")

	// Dos cambios de escena; el arranque del video no lleva efecto.
	quiero := []time.Duration{2 * time.Second, 3 * time.Second}
	if len(got) != len(quiero) {
		t.Fatalf("esperaba %d instantes, obtuve %d: %v", len(quiero), len(got), got)
	}
	for i := range quiero {
		if got[i] != quiero[i] {
			t.Errorf("instante %d = %v, esperaba %v", i, got[i], quiero[i])
		}
	}
}

func TestInstantesModoPlano(t *testing.T) {
	escenas := []proveedor.EscenaRender{{Imagenes: []string{"a", "b", "c"}}}
	got := instantesDeEfecto(escenas, tramosDe(3, time.Second), "plano")
	if len(got) != 2 {
		t.Fatalf("esperaba 2 instantes (3 imágenes, sin contar el arranque), obtuve %d", len(got))
	}
	if got[0] != time.Second {
		t.Errorf("primer instante = %v, esperaba 1s", got[0])
	}
}

func TestInstantesModoNinguno(t *testing.T) {
	escenas := []proveedor.EscenaRender{{Imagenes: []string{"a", "b"}}}
	if got := instantesDeEfecto(escenas, tramosDe(2, time.Second), "ninguno"); got != nil {
		t.Errorf("esperaba nil, obtuve %v", got)
	}
}

// Un video de una sola imagen no tiene cortes que marcar.
func TestInstantesConUnaSolaImagen(t *testing.T) {
	escenas := []proveedor.EscenaRender{{Imagenes: []string{"a"}}}
	if got := instantesDeEfecto(escenas, tramosDe(1, time.Second), "escena"); got != nil {
		t.Errorf("esperaba nil, obtuve %v", got)
	}
}
