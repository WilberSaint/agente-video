package proveedor

import "testing"

// Lo que hace que dos planos se vean como el mismo personaje: compartir
// semilla. Medido sobre un caso real, con semillas distintas el abrigo cambiaba
// de color y la cara salía deformada.
func TestPlanosDelMismoSujetoCompartenSemilla(t *testing.T) {
	a := Plano{Sujeto: "mujer"}
	b := Plano{Sujeto: "mujer"}

	// Escenas e índices distintos a propósito: solo debe importar el sujeto.
	if a.SemillaDe(1470, 1, 1) != b.SemillaDe(1470, 7, 0) {
		t.Errorf("dos planos de %q deberían compartir semilla: %d vs %d",
			a.Sujeto, a.SemillaDe(1470, 1, 1), b.SemillaDe(1470, 7, 0))
	}
}

func TestSujetosDistintosNoColisionan(t *testing.T) {
	mujer := Plano{Sujeto: "mujer"}.SemillaDe(1470, 1, 0)
	farero := Plano{Sujeto: "farero"}.SemillaDe(1470, 1, 0)
	if mujer == farero {
		t.Errorf("sujetos distintos comparten semilla %d", mujer)
	}
}

// El identificador puede llegar con mayúsculas o espacios sobrantes del modelo.
func TestSujetoSeNormaliza(t *testing.T) {
	limpio := Plano{Sujeto: "mujer"}.SemillaDe(1470, 1, 0)
	sucio := Plano{Sujeto: "  Mujer  "}.SemillaDe(1470, 5, 2)
	if limpio != sucio {
		t.Errorf("%q y %q deberían dar la misma semilla: %d vs %d",
			"mujer", "  Mujer  ", limpio, sucio)
	}
}

// Sin sujeto, cada plano varía: si no, dos prompts parecidos de la misma
// escena salen casi idénticos.
func TestSinSujetoLaSemillaVaria(t *testing.T) {
	p := Plano{}
	if p.SemillaDe(1470, 1, 0) == p.SemillaDe(1470, 1, 1) {
		t.Error("dos planos sin sujeto de la misma escena comparten semilla")
	}
	if p.SemillaDe(1470, 1, 0) == p.SemillaDe(1470, 2, 0) {
		t.Error("planos sin sujeto de escenas distintas comparten semilla")
	}
}

// Normalizar debe dejar utilizables los checkpoints del formato anterior, que
// traían un solo prompt por escena en vez de una lista de planos.
func TestNormalizarConvierteElFormatoAnterior(t *testing.T) {
	g := GuionGenerado{Escenas: []Escena{
		{Prompt: "un faro"},
		{Planos: []Plano{{Prompt: "una ola"}, {Prompt: "una roca"}}},
	}}
	g.Normalizar()

	if len(g.Escenas[0].Planos) != 1 {
		t.Fatalf("la escena antigua no se convirtió: %+v", g.Escenas[0])
	}
	if g.Escenas[0].Planos[0].Prompt != "un faro" {
		t.Errorf("prompt convertido = %q", g.Escenas[0].Planos[0].Prompt)
	}
	if g.Escenas[0].N != 1 || g.Escenas[1].N != 2 {
		t.Errorf("las escenas no quedaron numeradas: %d, %d", g.Escenas[0].N, g.Escenas[1].N)
	}
	if g.TotalPlanos() != 3 {
		t.Errorf("TotalPlanos = %d, esperaba 3", g.TotalPlanos())
	}
}

func TestNarracionCompletaOmiteVacias(t *testing.T) {
	g := GuionGenerado{Escenas: []Escena{
		{Narracion: "primera"}, {Narracion: "   "}, {Narracion: "segunda"},
	}}
	if got := g.NarracionCompleta(); got != "primera segunda" {
		t.Errorf("NarracionCompleta = %q", got)
	}
}
