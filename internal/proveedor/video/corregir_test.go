package video

import (
	"os"
	"strings"
	"testing"
	"time"
)

func palabrasDe(texto string) []palabra {
	var out []palabra
	for i, t := range strings.Fields(texto) {
		out = append(out, palabra{
			inicio: time.Duration(i) * time.Second,
			fin:    time.Duration(i+1) * time.Second,
			texto:  t,
		})
	}
	return out
}

func textoDe(ps []palabra) string {
	var out []string
	for _, p := range ps {
		out = append(out, p.texto)
	}
	return strings.Join(out, " ")
}

func TestCorregirArreglaLaPalabraMalOida(t *testing.T) {
	casos := []struct {
		nombre    string
		oido      string
		narracion string
		quiere    string
	}{
		{
			// El caso real: whisper escribió "rija" donde se dijo "hija".
			"una letra cambiada",
			"nadie supo que su rija seguia alli",
			"Nadie supo que su hija seguía allí",
			"Nadie supo que su hija seguía allí",
		},
		{
			// Whisper escribe cifras donde la narración lleva letras; en
			// pantalla debe verse lo que se dijo.
			"numero escrito con cifras",
			"tardo 40 anos en volver",
			"Tardó cuarenta años en volver",
			"Tardó cuarenta años en volver",
		},
		{
			"palabra que whisper se salto",
			"la casa donde relojes se paran",
			"la casa donde los relojes se paran",
			"la casa donde los relojes se paran",
		},
		{
			"palabra que whisper se invento",
			"el tren llegaba eh vacio cada noche",
			"el tren llegaba vacío cada noche",
			"el tren llegaba vacío cada noche",
		},
		{
			"sin errores no toca nada",
			"todo estaba en su sitio",
			"todo estaba en su sitio",
			"todo estaba en su sitio",
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, _, fiable := corregirConGuion(palabrasDe(c.oido), c.narracion)
			if !fiable {
				t.Fatal("se descartó la corrección siendo el texto correcto")
			}
			if s := textoDe(got); s != c.quiere {
				t.Errorf("quedó %q, se esperaba %q", s, c.quiere)
			}
		})
	}
}

func TestCorregirConservaLosTiempos(t *testing.T) {
	oido := palabrasDe("nadie supo que su rija seguia alli")
	got, _, fiable := corregirConGuion(oido, "Nadie supo que su hija seguía allí")
	if !fiable || len(got) != len(oido) {
		t.Fatalf("se esperaban %d palabras, hay %d", len(oido), len(got))
	}
	for i := range got {
		if got[i].inicio != oido[i].inicio || got[i].fin != oido[i].fin {
			t.Errorf("palabra %d: los tiempos cambiaron (%v-%v vs %v-%v)",
				i, got[i].inicio, got[i].fin, oido[i].inicio, oido[i].fin)
		}
	}
}

func TestCorregirSeAbstieneSiNoCuadran(t *testing.T) {
	// Otro audio, otro texto: imponer el guion daría subtítulos absurdos y
	// desincronizados. Es preferible dejar lo que oyó whisper.
	oido := palabrasDe("uno dos tres cuatro cinco seis")
	got, cambios, fiable := corregirConGuion(oido, "el faro que nadie visita desde entonces")
	if fiable {
		t.Error("debería haberse abstenido: los textos no tienen nada que ver")
	}
	if cambios != 0 || textoDe(got) != textoDe(oido) {
		t.Errorf("tocó los subtítulos pese a no fiarse: %q", textoDe(got))
	}
}

// El caso que lo destapó: whisper transcribió "su rija" en un video ya
// publicado. Los archivos son los de esa corrida, sin retocar.
func TestCorregirElVideoDelSotano(t *testing.T) {
	palabras, err := leerSRT("testdata/sotano.srt")
	if err != nil {
		t.Fatal(err)
	}
	narracion, err := os.ReadFile("testdata/sotano.txt")
	if err != nil {
		t.Fatal(err)
	}

	got, cambios, fiable := corregirConGuion(palabras, string(narracion))
	if !fiable {
		t.Fatal("se abstuvo con una transcripción que sí corresponde al audio")
	}
	texto := textoDe(got)
	if strings.Contains(strings.ToLower(texto), "rija") {
		t.Error("sigue apareciendo «rija» en los subtítulos")
	}
	if !strings.Contains(texto, "hija") {
		t.Error("no aparece «hija», que es lo que decía la narración")
	}
	t.Logf("%d palabra(s) corregidas de %d", cambios, len(palabras))
}

// Los errores concretos que whisper cometió en ese video. Se dejan escritos
// uno a uno porque son los que demuestran para qué sirve esto: no es una
// mejora cosmética, son palabras equivocadas en pantalla.
func TestCorregirArreglaLosErroresConcretosDelSotano(t *testing.T) {
	palabras, err := leerSRT("testdata/sotano.srt")
	if err != nil {
		t.Fatal(err)
	}
	narracion, err := os.ReadFile("testdata/sotano.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _, fiable := corregirConGuion(palabras, string(narracion))
	if !fiable {
		t.Fatal("se abstuvo con una transcripción que sí corresponde al audio")
	}
	texto := textoDe(got)

	// Palabra completa: "medad" vive dentro de "humedad", que es justo la
	// corrección, así que buscarlo como subcadena daría un falso positivo.
	sueltas := map[string]bool{}
	for _, w := range strings.Fields(texto) {
		sueltas[normalizar(w)] = true
	}
	for _, mal := range []string{"rija", "medad", "propa", "visieras", "aparrado"} {
		if sueltas[mal] {
			t.Errorf("sigue apareciendo %q, que whisper oyó mal", mal)
		}
	}
	for _, bien := range []string{"hija", "humedad", "Ropa", "hicieras", "apagado",
		"Esperabas", "setenta", "noventa y ocho"} {
		if !strings.Contains(texto, bien) {
			t.Errorf("falta %q, que es lo que decía la narración", bien)
		}
	}
}
