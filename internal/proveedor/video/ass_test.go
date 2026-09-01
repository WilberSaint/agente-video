package video

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agente-video/internal/perfil"
)

const srtDePrueba = `1
00:00:00,000 --> 00:00:00,090


2
00:00:00,090 --> 00:00:00,210
 Hay

3
00:00:00,210 --> 00:00:00,350
 un

4
00:00:00,350 --> 00:00:00,630
 faro

5
00:00:02,000 --> 00:00:02,400
 Nadie
`

func escribirTemp(t *testing.T, contenido string) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "subs.srt")
	if err := os.WriteFile(ruta, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func TestLeerSRTOmiteEntradasVacias(t *testing.T) {
	palabras, err := leerSRT(escribirTemp(t, srtDePrueba))
	if err != nil {
		t.Fatal(err)
	}
	// La entrada 1 no tiene texto (silencio) y no debe convertirse en subtítulo.
	if len(palabras) != 4 {
		t.Fatalf("esperaba 4 palabras, obtuve %d: %+v", len(palabras), palabras)
	}
	if palabras[0].texto != "Hay" {
		t.Errorf("primera palabra = %q, esperaba %q", palabras[0].texto, "Hay")
	}
	if palabras[0].inicio != 90*time.Millisecond {
		t.Errorf("inicio = %v, esperaba 90ms", palabras[0].inicio)
	}
	if palabras[2].fin != 630*time.Millisecond {
		t.Errorf("fin de la tercera = %v, esperaba 630ms", palabras[2].fin)
	}
}

func TestAgruparCortaEnPausaLarga(t *testing.T) {
	palabras, err := leerSRT(escribirTemp(t, srtDePrueba))
	if err != nil {
		t.Fatal(err)
	}
	// Con tope de 10 no se llena ninguna línea: el único corte posible es la
	// pausa de 1.37s que hay antes de "Nadie".
	grupos := agrupar(palabras, 10)
	if len(grupos) != 2 {
		t.Fatalf("esperaba 2 grupos por la pausa, obtuve %d", len(grupos))
	}
	if got := grupos[0].texto(); got != "Hay un faro" {
		t.Errorf("grupo 1 = %q", got)
	}
	if got := grupos[1].texto(); got != "Nadie" {
		t.Errorf("grupo 2 = %q", got)
	}
}

func TestAgruparRespetaTopePorLinea(t *testing.T) {
	palabras, err := leerSRT(escribirTemp(t, srtDePrueba))
	if err != nil {
		t.Fatal(err)
	}
	grupos := agrupar(palabras, 2)
	// "Hay un" llena el tope; "faro" queda solo porque después viene la pausa
	// larga, y "Nadie" cierra al final. Los dos criterios de corte conviven.
	quiero := []string{"Hay un", "faro", "Nadie"}
	if len(grupos) != len(quiero) {
		t.Fatalf("esperaba %d grupos, obtuve %d", len(quiero), len(grupos))
	}
	for i, q := range quiero {
		if got := grupos[i].texto(); got != q {
			t.Errorf("grupo %d = %q, esperaba %q", i+1, got, q)
		}
	}
}

func TestTiempoASS(t *testing.T) {
	casos := []struct {
		d      time.Duration
		quiero string
	}{
		{0, "0:00:00.00"},
		{90 * time.Millisecond, "0:00:00.09"},
		{2*time.Minute + 3*time.Second + 456*time.Millisecond, "0:02:03.45"},
		{time.Hour + 30*time.Second, "1:00:30.00"},
		{-5 * time.Second, "0:00:00.00"}, // nunca negativo
	}
	for _, c := range casos {
		if got := tiempoASS(c.d); got != c.quiero {
			t.Errorf("tiempoASS(%v) = %q, esperaba %q", c.d, got, c.quiero)
		}
	}
}

func TestColorASSQuitaElAmpersandFinal(t *testing.T) {
	// force_style usa &H...& pero dentro de un archivo ASS el & final sobra.
	casos := map[string]string{
		"&H00FFFFFF&": "&H00FFFFFF",
		"&H0000E5FF":  "&H0000E5FF",
		"":            "&H00FFFFFF",
		"#00FF00":     "&H00FF00",
	}
	for entrada, quiero := range casos {
		if got := colorASS(entrada); got != quiero {
			t.Errorf("colorASS(%q) = %q, esperaba %q", entrada, got, quiero)
		}
	}
}

func perfilDePrueba(animacion string) *perfil.Perfil {
	return &perfil.Perfil{
		Formato: perfil.Formato{Ancho: 1080, Alto: 1920, FPS: 30},
		Subtitulos: perfil.Subtitulos{
			Activos: true, TamPx: 78, MargenV: 300, GrosorBorde: 5,
			ColorPrimario: "&H00FFFFFF&", ColorBorde: "&H00000000&",
			ColorActivo: "&H0000E5FF&", EscalaPop: 112,
			Animacion: animacion, PalabrasPorLinea: 4,
		},
	}
}

// La razón de ser de este archivo: el PlayRes debe coincidir con el video, o
// los márgenes en píxeles empujan el texto fuera del cuadro sin ningún error.
func TestGenerarASSUsaLaResolucionDelVideo(t *testing.T) {
	destino := filepath.Join(t.TempDir(), "out.ass")
	if err := generarASS(escribirTemp(t, srtDePrueba), destino, perfilDePrueba("pop"), "Arial", "", nil); err != nil {
		t.Fatal(err)
	}
	datos, err := os.ReadFile(destino)
	if err != nil {
		t.Fatal(err)
	}
	ass := string(datos)
	for _, quiero := range []string{"PlayResX: 1080", "PlayResY: 1920"} {
		if !strings.Contains(ass, quiero) {
			t.Errorf("falta %q en el ASS generado", quiero)
		}
	}
}

func TestGenerarASSPorModo(t *testing.T) {
	casos := []struct {
		animacion    string
		minEventos   int
		debeContener string
	}{
		{"ninguna", 1, "Dialogue:"},
		{"pop", 1, `\fscx112\fscy112`},  // el rebote
		{"karaoke", 4, `{\c&H0000E5FF`}, // color de palabra activa
		// En "palabra" son 3 y no 4 porque "un" se une a "faro": las palabras
		// vacías nunca aparecen solas. Ver TestPalabrasVaciasNoSalenSolas.
		{"palabra", 3, `\fad(60,50)`},
	}
	for _, c := range casos {
		t.Run(c.animacion, func(t *testing.T) {
			destino := filepath.Join(t.TempDir(), "out.ass")
			err := generarASS(escribirTemp(t, srtDePrueba), destino,
				perfilDePrueba(c.animacion), "Arial", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			datos, _ := os.ReadFile(destino)
			ass := string(datos)

			eventos := strings.Count(ass, "Dialogue:")
			if eventos < c.minEventos {
				t.Errorf("%d eventos, esperaba al menos %d", eventos, c.minEventos)
			}
			if !strings.Contains(ass, c.debeContener) {
				t.Errorf("el ASS no contiene %q", c.debeContener)
			}
		})
	}
}

// Una transcripción con llaves rompería el renderizado, porque en ASS delimitan
// las etiquetas de override.
func TestEscaparASSNeutralizaLlaves(t *testing.T) {
	if got := escaparASS("un {mal} texto"); strings.ContainsAny(got, "{}") {
		t.Errorf("quedaron llaves sin neutralizar: %q", got)
	}
}

func TestGenerarASSFallaConSRTVacio(t *testing.T) {
	destino := filepath.Join(t.TempDir(), "out.ass")
	err := generarASS(escribirTemp(t, "\n"), destino, perfilDePrueba("pop"), "Arial", "", nil)
	if err == nil {
		t.Fatal("esperaba error con un SRT sin subtítulos")
	}
}

// En español la mitad de las palabras son artículos y preposiciones de dos o
// tres letras. Mostrarlas solas a pantalla completa se lee como un error, no
// como un subtítulo: se comprobó viendo "el" solo en medio de un fotograma.
func TestPalabrasVaciasNoSalenSolas(t *testing.T) {
	entrada := []palabra{
		{texto: "Al", inicio: 0, fin: 100 * time.Millisecond},
		{texto: "final", inicio: 100 * time.Millisecond, fin: 400 * time.Millisecond},
		{texto: "no", inicio: 400 * time.Millisecond, fin: 500 * time.Millisecond},
		{texto: "construyes", inicio: 500 * time.Millisecond, fin: 900 * time.Millisecond},
		{texto: "un", inicio: 900 * time.Millisecond, fin: 1000 * time.Millisecond},
		{texto: "cuerpo", inicio: 1000 * time.Millisecond, fin: 1400 * time.Millisecond},
	}
	got := unirPalabrasVacias(entrada)

	quiero := []string{"Al final", "no construyes", "un cuerpo"}
	if len(got) != len(quiero) {
		t.Fatalf("%d grupos, esperaba %d: %+v", len(got), len(quiero), got)
	}
	for i, q := range quiero {
		if got[i].texto != q {
			t.Errorf("grupo %d = %q, esperaba %q", i, got[i].texto, q)
		}
	}
	// La sincronía debe conservarse: cada grupo empieza con su primera palabra
	// y acaba con la última.
	if got[0].inicio != 0 || got[0].fin != 400*time.Millisecond {
		t.Errorf("tiempos del primer grupo = %v-%v", got[0].inicio, got[0].fin)
	}
}

func TestVariasVaciasSeguidasSeAcumulan(t *testing.T) {
	entrada := []palabra{
		{texto: "y", inicio: 0, fin: 100 * time.Millisecond},
		{texto: "de", inicio: 100 * time.Millisecond, fin: 200 * time.Millisecond},
		{texto: "la", inicio: 200 * time.Millisecond, fin: 300 * time.Millisecond},
		{texto: "montaña", inicio: 300 * time.Millisecond, fin: 700 * time.Millisecond},
	}
	got := unirPalabrasVacias(entrada)
	if len(got) != 1 || got[0].texto != "y de la montaña" {
		t.Fatalf("obtuve %+v", got)
	}
}

// Si el texto acaba en palabra vacía no hay a quién pegarla; se une a la
// anterior antes que descartarla, porque perder texto es peor que un grupo largo.
func TestVaciaFinalSeUneALaAnterior(t *testing.T) {
	entrada := []palabra{
		{texto: "camina", inicio: 0, fin: 400 * time.Millisecond},
		{texto: "más", inicio: 400 * time.Millisecond, fin: 600 * time.Millisecond},
	}
	got := unirPalabrasVacias(entrada)
	if len(got) != 1 || got[0].texto != "camina más" {
		t.Fatalf("obtuve %+v", got)
	}
	if got[0].fin != 600*time.Millisecond {
		t.Errorf("el fin no se extendió: %v", got[0].fin)
	}
}

// La puntuación no debe impedir reconocer una palabra vacía.
func TestVaciaConPuntuacion(t *testing.T) {
	entrada := []palabra{
		{texto: "¿Y", inicio: 0, fin: 100 * time.Millisecond},
		{texto: "ahora?", inicio: 100 * time.Millisecond, fin: 500 * time.Millisecond},
	}
	got := unirPalabrasVacias(entrada)
	if len(got) != 1 || got[0].texto != "¿Y ahora?" {
		t.Fatalf("obtuve %+v", got)
	}
}

// Comprobación de fondo sobre el ASS ya generado: ningún subtítulo del modo
// palabra puede consistir en una sola palabra vacía. Es lo que se vio mal en un
// video real, y conviene vigilarlo sobre la salida y no solo sobre la función.
func TestElASSNoMuestraArticulosSolos(t *testing.T) {
	srt := `1
00:00:00,000 --> 00:00:00,300
 Al

2
00:00:00,300 --> 00:00:00,800
 final

3
00:00:00,800 --> 00:00:01,000
 de

4
00:00:01,000 --> 00:00:01,600
 todo
`
	destino := filepath.Join(t.TempDir(), "out.ass")
	if err := generarASS(escribirTemp(t, srt), destino, perfilDePrueba("palabra"), "Arial", "", nil); err != nil {
		t.Fatal(err)
	}
	datos, _ := os.ReadFile(destino)

	for _, linea := range strings.Split(string(datos), "\n") {
		if !strings.HasPrefix(linea, "Dialogue:") {
			continue
		}
		partes := strings.SplitN(linea, ",,", 2)
		if len(partes) != 2 {
			continue
		}
		// Quitar las etiquetas de override para quedarnos con el texto visible.
		texto := partes[1]
		for {
			ini := strings.Index(texto, "{")
			fin := strings.Index(texto, "}")
			if ini < 0 || fin < ini {
				break
			}
			texto = texto[:ini] + texto[fin+1:]
		}
		texto = strings.TrimSpace(texto)
		if palabrasVacias[strings.ToLower(texto)] {
			t.Errorf("el subtítulo %q es una palabra vacía sola", texto)
		}
	}
}
