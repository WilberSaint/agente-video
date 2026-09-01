package proveedor

import "strings"

import "testing"

func TestEtiquetasSiempreIncluyeViral(t *testing.T) {
	casos := []struct {
		nombre string
		dadas  []string
		quiere string
	}{
		{"el modelo no la puso", []string{"historia", "misterio"}, "#historia #misterio #viral"},
		{"sin hashtags", nil, "#viral"},
		{"ya venía con almohadilla", []string{"#historia"}, "#historia #viral"},
		{"el modelo ya la puso: no se duplica", []string{"viral", "historia"}, "#viral #historia"},
		{"la puso con almohadilla y mayúsculas", []string{"#Viral"}, "#viral"},
		{"repetidas", []string{"habitos", "Hábitos", "habitos"}, "#habitos #viral"},
		{"con espacios y acentos", []string{"salud mental", "reflexión"}, "#saludmental #reflexion #viral"},
		{"basura que queda vacía", []string{"###", "  ", "ok"}, "#ok #viral"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			g := &GuionGenerado{Hashtags: c.dadas}
			if got := strings.Join(g.Etiquetas(), " "); got != c.quiere {
				t.Errorf("Etiquetas() = %q, se esperaba %q", got, c.quiere)
			}
		})
	}
}

func TestTituloPublicableLlevaViral(t *testing.T) {
	g := &GuionGenerado{Titulo: "Por qué la constancia vence al talento"}
	if got := g.TituloPublicable(); got != "Por qué la constancia vence al talento #viral" {
		t.Errorf("TituloPublicable() = %q", got)
	}
	// Si el modelo desobedece y ya lo puso, no queremos "#viral #viral".
	ya := &GuionGenerado{Titulo: "Algo #viral"}
	if got := ya.TituloPublicable(); got != "Algo #viral" {
		t.Errorf("se duplicó la etiqueta: %q", got)
	}
}

func TestDescripcionPublicableLlevaLasEtiquetasDebajo(t *testing.T) {
	g := &GuionGenerado{
		Descripcion: "Una idea corta para quien insiste en silencio.",
		Hashtags:    []string{"constancia", "disciplina"},
	}
	quiere := "Una idea corta para quien insiste en silencio.\n\n#constancia #disciplina #viral"
	if got := g.DescripcionPublicable(); got != quiere {
		t.Errorf("DescripcionPublicable() = %q, se esperaba %q", got, quiere)
	}
	// Sin descripción no debe quedar un hueco al principio del texto.
	vacia := &GuionGenerado{Hashtags: []string{"a"}}
	if got := vacia.DescripcionPublicable(); got != "#a #viral" {
		t.Errorf("descripción vacía = %q", got)
	}
}
