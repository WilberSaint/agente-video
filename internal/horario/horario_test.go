package horario

import (
	"testing"
	"time"
)

// enPunto construye un instante local para las pruebas.
func enPunto(dia int, hh, mm int) time.Time {
	// Enero de 2026: el día 1 fue jueves, así que el 4 es domingo.
	return time.Date(2026, 1, dia, hh, mm, 0, 0, time.Local)
}

func conRegla(r *Regla) *Horario {
	return &Horario{reglas: []*Regla{r}}
}

func TestDisparaALaHora(t *testing.T) {
	h := conRegla(&Regla{ID: "a", Activa: true, Perfil: "demo", Hora: "03:00", Cantidad: 2})

	if n := len(h.pendientesDeDisparar(enPunto(5, 2, 59))); n != 0 {
		t.Errorf("disparó un minuto antes de la hora")
	}
	if n := len(h.pendientesDeDisparar(enPunto(5, 3, 0))); n != 1 {
		t.Errorf("no disparó a la hora exacta")
	}
}

// Ventana de gracia: si el servidor estuvo caído a la hora prevista, todavía se
// recupera dentro de la hora siguiente. Más tarde se deja pasar el día, porque
// producir a destiempo es peor que no producir.
func TestVentanaDeGracia(t *testing.T) {
	h := conRegla(&Regla{ID: "a", Activa: true, Perfil: "demo", Hora: "03:00", Cantidad: 1})

	if n := len(h.pendientesDeDisparar(enPunto(5, 3, 45))); n != 1 {
		t.Errorf("no recuperó el disparo dentro de la hora de gracia")
	}
	if n := len(h.pendientesDeDisparar(enPunto(5, 4, 30))); n != 0 {
		t.Errorf("disparó hora y media tarde; debería dejarlo pasar")
	}
}

// Lo que evita duplicar la producción de una noche: se compara por fecha y no
// por intervalo, así un cambio de hora o un salto del reloj no provoca un
// segundo disparo.
func TestNoDisparaDosVecesElMismoDia(t *testing.T) {
	ayer := enPunto(5, 3, 0)
	h := conRegla(&Regla{
		ID: "a", Activa: true, Perfil: "demo", Hora: "03:00", Cantidad: 1,
		UltimoDisparo: &ayer,
	})

	if n := len(h.pendientesDeDisparar(enPunto(5, 3, 30))); n != 0 {
		t.Errorf("disparó dos veces el mismo día")
	}
	if n := len(h.pendientesDeDisparar(enPunto(6, 3, 0))); n != 1 {
		t.Errorf("no disparó al día siguiente")
	}
}

func TestRespetaLosDiasDeLaSemana(t *testing.T) {
	// Solo lunes (1) y miércoles (3).
	h := conRegla(&Regla{
		ID: "a", Activa: true, Perfil: "demo", Hora: "03:00", Cantidad: 1,
		Dias: []int{1, 3},
	})

	// 5 de enero de 2026 es lunes; el 6, martes.
	if got := enPunto(5, 3, 0).Weekday(); got != time.Monday {
		t.Fatalf("la prueba asume que el 5/1/2026 es lunes, pero es %v", got)
	}
	if n := len(h.pendientesDeDisparar(enPunto(5, 3, 0))); n != 1 {
		t.Errorf("no disparó un lunes estando en la lista")
	}
	if n := len(h.pendientesDeDisparar(enPunto(6, 3, 0))); n != 0 {
		t.Errorf("disparó un martes, que no está en la lista")
	}
}

func TestReglaDesactivadaNoDispara(t *testing.T) {
	h := conRegla(&Regla{ID: "a", Activa: false, Perfil: "demo", Hora: "03:00", Cantidad: 1})
	if n := len(h.pendientesDeDisparar(enPunto(5, 3, 0))); n != 0 {
		t.Errorf("una regla desactivada disparó")
	}
}

// Una regla con hora ilegible no debe tumbar el vigilante ni disparar a ciegas.
func TestHoraInvalidaSeIgnora(t *testing.T) {
	h := conRegla(&Regla{ID: "a", Activa: true, Perfil: "demo", Hora: "las tres", Cantidad: 1})
	if n := len(h.pendientesDeDisparar(enPunto(5, 3, 0))); n != 0 {
		t.Errorf("una hora inválida provocó un disparo")
	}
}

func TestGuardarValida(t *testing.T) {
	h := Nuevo("", nil)
	casos := []struct {
		nombre string
		r      Regla
	}{
		{"hora ilegible", Regla{Perfil: "demo", Hora: "25:99", Cantidad: 1}},
		{"sin perfil", Regla{Hora: "03:00", Cantidad: 1}},
		{"cantidad cero", Regla{Perfil: "demo", Hora: "03:00", Cantidad: 0}},
	}
	for _, c := range casos {
		if _, err := h.Guardar(c.r); err == nil {
			t.Errorf("aceptó una regla con %s", c.nombre)
		}
	}
	if _, err := h.Guardar(Regla{Perfil: "demo", Hora: "03:00", Cantidad: 2}); err != nil {
		t.Errorf("rechazó una regla válida: %v", err)
	}
}

// Al editar desde el panel el formulario no lleva el histórico; perderlo haría
// que la regla volviera a dispararse el mismo día.
func TestEditarConservaElHistorico(t *testing.T) {
	h := Nuevo("", nil)
	creada, err := h.Guardar(Regla{Perfil: "demo", Hora: "03:00", Cantidad: 1, Activa: true})
	if err != nil {
		t.Fatal(err)
	}
	ayer := enPunto(5, 3, 0)
	h.reglas[0].UltimoDisparo = &ayer
	h.reglas[0].UltimoResumen = "2 videos encolados"

	editada := *creada
	editada.Cantidad = 3
	if _, err := h.Guardar(editada); err != nil {
		t.Fatal(err)
	}

	if h.reglas[0].UltimoDisparo == nil {
		t.Error("se perdió la marca del último disparo al editar")
	}
	if h.reglas[0].UltimoResumen == "" {
		t.Error("se perdió el resumen del último disparo al editar")
	}
	if h.reglas[0].Cantidad != 3 {
		t.Errorf("no se aplicó el cambio: cantidad = %d", h.reglas[0].Cantidad)
	}
}
