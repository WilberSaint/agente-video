package video

import (
	"testing"
	"time"

	"agente-video/internal/proveedor"
)

func seg(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// palabrasUniformes simula una transcripción de n palabras repartidas parejo,
// que es suficiente para comprobar el reparto proporcional.
func palabrasUniformes(n int, total time.Duration) []palabra {
	ps := make([]palabra, n)
	paso := total / time.Duration(n)
	for i := range ps {
		ps[i] = palabra{
			inicio: time.Duration(i) * paso,
			fin:    time.Duration(i+1) * paso,
			texto:  "x",
		}
	}
	return ps
}

func escena(narracion string, imagenes ...string) proveedor.EscenaRender {
	return proveedor.EscenaRender{Narracion: narracion, Imagenes: imagenes}
}

// Lo que motiva todo el archivo: una escena que se narra durante más tiempo
// debe tener sus imágenes en pantalla más tiempo. Con reparto uniforme las
// imágenes cambian a destiempo del relato.
func TestRepartirSigueElPesoDeLaNarracion(t *testing.T) {
	total := seg(30)
	escenas := []proveedor.EscenaRender{
		escena("una dos tres cuatro cinco seis siete ocho nueve diez once doce", "a.jpg"),
		escena("corta", "b.jpg"),
	}
	tramos := repartir(escenas, palabrasUniformes(13, total), total, 0, 0)

	if len(tramos) != 2 {
		t.Fatalf("esperaba 2 tramos, obtuve %d", len(tramos))
	}
	if tramos[0].dura() <= tramos[1].dura() {
		t.Errorf("la escena larga (%v) debería durar más que la corta (%v)",
			tramos[0].dura(), tramos[1].dura())
	}
}

func TestRepartirCubreExactamenteElAudio(t *testing.T) {
	total := seg(24)
	escenas := []proveedor.EscenaRender{
		escena("uno dos tres", "a.jpg", "b.jpg"),
		escena("cuatro cinco", "c.jpg"),
		escena("seis siete ocho nueve", "d.jpg", "e.jpg", "f.jpg"),
	}
	tramos := repartir(escenas, palabrasUniformes(9, total), total, 1.0, 8.0)

	if len(tramos) != 6 {
		t.Fatalf("esperaba 6 tramos (una por imagen), obtuve %d", len(tramos))
	}
	if tramos[0].inicio != 0 {
		t.Errorf("el primer tramo empieza en %v, esperaba 0", tramos[0].inicio)
	}
	if tramos[len(tramos)-1].fin != total {
		t.Errorf("el último tramo termina en %v, esperaba %v", tramos[len(tramos)-1].fin, total)
	}
	// Sin huecos ni solapes: cada tramo arranca donde termina el anterior.
	for i := 1; i < len(tramos); i++ {
		if tramos[i].inicio != tramos[i-1].fin {
			t.Errorf("hueco entre el tramo %d y el %d: %v vs %v",
				i-1, i, tramos[i-1].fin, tramos[i].inicio)
		}
	}
}

// Una escena larga con un solo plano dejaría la imagen congelada; el tope la
// recorta y el resto del tiempo se redistribuye.
func TestRepartirRespetaElTopeMaximo(t *testing.T) {
	total := seg(40)
	escenas := []proveedor.EscenaRender{
		escena("una dos tres cuatro cinco seis siete ocho nueve diez once doce trece catorce", "larga.jpg"),
		escena("corta", "a.jpg"),
		escena("corta", "b.jpg"),
	}
	sinTope := repartir(escenas, palabrasUniformes(16, total), total, 0, 0)
	conTope := repartir(escenas, palabrasUniformes(16, total), total, 0, 6.0)

	if sinTope[0].dura() <= seg(6) {
		t.Skip("el caso de prueba no llega al tope; nada que comprobar")
	}
	if conTope[0].dura() >= sinTope[0].dura() {
		t.Errorf("con tope la primera imagen dura %v, sin tope %v: debería reducirse",
			conTope[0].dura(), sinTope[0].dura())
	}
	if conTope[len(conTope)-1].fin != total {
		t.Errorf("tras aplicar el tope el total quedó en %v, esperaba %v",
			conTope[len(conTope)-1].fin, total)
	}
}

// Si los subtítulos están desactivados no hay tiempos por palabra y lo único
// posible es repartir parejo. No debe reventar.
func TestRepartirSinTiemposCaeAUniforme(t *testing.T) {
	total := seg(12)
	escenas := []proveedor.EscenaRender{
		escena("lo que sea", "a.jpg", "b.jpg"),
		escena("otra cosa", "c.jpg"),
	}
	tramos := repartir(escenas, nil, total, 0, 0)
	if len(tramos) != 3 {
		t.Fatalf("esperaba 3 tramos, obtuve %d", len(tramos))
	}
	esperado := seg(4)
	for i, tr := range tramos {
		if d := tr.dura(); d < esperado-time.Millisecond || d > esperado+time.Millisecond {
			t.Errorf("tramo %d dura %v, esperaba ~%v", i, d, esperado)
		}
	}
}

// Con muchísimas imágenes el mínimo es imposible de cumplir; manda el audio.
func TestRepartirConMinimoImposible(t *testing.T) {
	total := seg(5)
	var imgs []string
	for i := 0; i < 20; i++ {
		imgs = append(imgs, "x.jpg")
	}
	tramos := repartir([]proveedor.EscenaRender{escena("texto", imgs...)},
		palabrasUniformes(4, total), total, 3.0, 8.0)

	if len(tramos) != 20 {
		t.Fatalf("esperaba 20 tramos, obtuve %d", len(tramos))
	}
	if tramos[len(tramos)-1].fin != total {
		t.Errorf("el total quedó en %v, esperaba %v", tramos[len(tramos)-1].fin, total)
	}
}

func TestRepartirSinImagenes(t *testing.T) {
	if tramos := repartir(nil, nil, seg(10), 0, 0); tramos != nil {
		t.Errorf("sin escenas esperaba nil, obtuve %v", tramos)
	}
}

func TestZoomSegunEncuadre(t *testing.T) {
	base := 1.20
	if z := zoomDeEncuadre(base, "general"); z != base {
		t.Errorf("un plano general debería usar el zoom base: %v", z)
	}
	cercano := zoomDeEncuadre(base, "cercano")
	if cercano >= base || cercano <= 1.0 {
		t.Errorf("un plano cercano debería moverse menos pero moverse: %v", cercano)
	}
	if zoomDeEncuadre(base, "") != base {
		t.Error("sin encuadre debería usarse el zoom base")
	}
}

func TestContarPalabras(t *testing.T) {
	casos := map[string]int{
		"":                     0,
		"una":                  1,
		"  dos   palabras  ":   2,
		"tres\npalabras\taquí": 3,
	}
	for entrada, quiero := range casos {
		if got := contarPalabras(entrada); got != quiero {
			t.Errorf("contarPalabras(%q) = %d, esperaba %d", entrada, got, quiero)
		}
	}
}
