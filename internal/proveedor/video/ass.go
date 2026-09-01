package video

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"agente-video/internal/perfil"
)

// Este archivo genera el ASS de subtítulos. Existe por dos razones.
//
// La primera: el filtro subtitles= de ffmpeg convierte el SRT a ASS sobre un
// lienzo fijo de 384x288, y force_style se interpreta en ESE espacio, no en
// píxeles del video. Con un video de 1080x1920, un MarginV=300 empuja el texto
// fuera de un lienzo de 288 de alto: los subtítulos desaparecen, sin error y
// con ffmpeg saliendo en 0. Generándolo nosotros con PlayRes igual al video,
// cada valor del perfil está en píxeles reales.
//
// La segunda: ASS permite animar con etiquetas de override, y ahí es donde se
// consiguen los efectos de subtítulo que se usan en video vertical. whisper nos
// da tiempos por palabra, así que podemos animar palabra por palabra sin
// depender de ninguna librería extra.

type palabra struct {
	inicio time.Duration
	fin    time.Duration
	texto  string
}

// grupo es una línea de subtítulo: varias palabras que se muestran juntas.
type grupo struct {
	palabras []palabra
}

func (g grupo) inicio() time.Duration { return g.palabras[0].inicio }
func (g grupo) fin() time.Duration    { return g.palabras[len(g.palabras)-1].fin }

func (g grupo) texto() string {
	partes := make([]string, len(g.palabras))
	for i, p := range g.palabras {
		partes[i] = p.texto
	}
	return strings.Join(partes, " ")
}

// generarASS convierte el SRT palabra-por-palabra de whisper en un ASS
// con el estilo y la animación que pide el perfil.
// narracion es el texto que se mandó a sintetizar. Si viene, manda sobre lo
// que oyó whisper; si viene vacío, se usa la transcripción tal cual.
func generarASS(rutaSRT, destino string, p *perfil.Perfil, fuente, narracion string, aviso func(string, ...any)) error {
	palabras, err := leerSRT(rutaSRT)
	if err != nil {
		return err
	}
	if len(palabras) == 0 {
		return fmt.Errorf("%s no contiene subtítulos", rutaSRT)
	}
	if narracion != "" {
		corregidas, cambios, fiable := corregirConGuion(palabras, narracion)
		switch {
		case !fiable:
			if aviso != nil {
				aviso("la transcripción no cuadra con el guion; se usan los subtítulos tal cual")
			}
		case cambios > 0:
			palabras = corregidas
			if aviso != nil {
				aviso("%d palabra(s) de los subtítulos corregidas con el guion", cambios)
			}
		default:
			palabras = corregidas
		}
	}

	s := p.Subtitulos
	grupos := agrupar(palabras, s.PalabrasPorLinea)

	var b strings.Builder
	escribirCabecera(&b, p, fuente)

	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	switch s.Animacion {
	case "palabra":
		eventosPalabraAPalabra(&b, palabras, s)
	case "karaoke":
		eventosKaraoke(&b, grupos, s)
	case "pop":
		eventosPop(&b, grupos, s)
	default: // "ninguna" o vacío
		for _, g := range grupos {
			dialogo(&b, g.inicio(), g.fin(), "", g.texto())
		}
	}

	return os.WriteFile(destino, []byte(b.String()), 0o644)
}

func escribirCabecera(b *strings.Builder, p *perfil.Perfil, fuente string) {
	s := p.Subtitulos
	// Márgenes laterales generosos: en vertical el texto no debe tocar el borde.
	margenLateral := p.Formato.Ancho / 12

	fmt.Fprintf(b, "[Script Info]\n")
	fmt.Fprintf(b, "; generado por agente-video\n")
	fmt.Fprintf(b, "ScriptType: v4.00+\n")
	fmt.Fprintf(b, "PlayResX: %d\n", p.Formato.Ancho)
	fmt.Fprintf(b, "PlayResY: %d\n", p.Formato.Alto)
	fmt.Fprintf(b, "ScaledBorderAndShadow: yes\n")
	fmt.Fprintf(b, "WrapStyle: 0\n\n")

	fmt.Fprintf(b, "[V4+ Styles]\n")
	fmt.Fprintf(b, "Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, "+
		"OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, "+
		"Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, "+
		"MarginV, Encoding\n")
	fmt.Fprintf(b, "Style: Default,%s,%d,%s,%s,%s,&H64000000,-1,0,0,0,100,100,0,0,1,%d,%d,2,%d,%d,%d,1\n\n",
		fuente,
		s.TamPx,
		colorASS(s.ColorPrimario),
		colorASS(s.ColorActivo),
		colorASS(s.ColorBorde),
		s.GrosorBorde,
		s.GrosorBorde/2, // una sombra leve despega el texto del fondo
		margenLateral, margenLateral,
		s.MargenV)
}

// eventosPop hace que cada línea entre con un rebote de escala: aparece
// pequeña, se pasa un poco de tamaño y se asienta. Es el efecto que hace que
// el subtítulo se sienta "vivo" en lugar de aparecer de golpe.
func eventosPop(b *strings.Builder, grupos []grupo, s perfil.Subtitulos) {
	for _, g := range grupos {
		dialogo(b, g.inicio(), g.fin(), tagsPop(s), g.texto())
	}
}

// eventosPalabraAPalabra muestra una palabra a la vez, con el mismo rebote. Es
// el formato más agresivo y el que mejor retiene en vertical.
func eventosPalabraAPalabra(b *strings.Builder, palabras []palabra, s perfil.Subtitulos) {
	grupos := unirPalabrasVacias(palabras)
	for i, g := range grupos {
		fin := g.fin
		// Estirar hasta la siguiente evita parpadeos entre huecos cortos, sin
		// dejar una palabra colgada durante un silencio largo.
		if i+1 < len(grupos) {
			if hueco := grupos[i+1].inicio - g.fin; hueco > 0 && hueco < 400*time.Millisecond {
				fin = grupos[i+1].inicio
			}
		}
		dialogo(b, g.inicio, fin, tagsPop(s), g.texto)
	}
}

// palabrasVacias son las que no dicen nada por sí solas. En español son
// muchísimas y muy cortas, y mostrarlas aisladas a pantalla completa —"el",
// "de", "que"— se lee como un error, no como un subtítulo.
var palabrasVacias = map[string]bool{
	"el": true, "la": true, "los": true, "las": true, "un": true, "una": true,
	"unos": true, "unas": true, "de": true, "del": true, "al": true, "a": true,
	"en": true, "y": true, "o": true, "que": true, "se": true, "su": true,
	"sus": true, "lo": true, "le": true, "les": true, "con": true, "por": true,
	"para": true, "es": true, "no": true, "ni": true, "mi": true, "tu": true,
	"me": true, "te": true, "sin": true, "ya": true, "si": true, "más": true,
}

// unirPalabrasVacias pega cada palabra vacía a la siguiente, de modo que nunca
// aparece sola en pantalla. Se conservan los tiempos: empieza cuando empieza la
// primera y acaba cuando acaba la última, así la sincronía con la voz se mantiene.
func unirPalabrasVacias(palabras []palabra) []palabra {
	var out []palabra
	var pendiente *palabra

	for _, p := range palabras {
		limpia := strings.ToLower(strings.Trim(p.texto, ".,;:¿?¡!—-«»\"'"))

		if pendiente != nil {
			pendiente.texto += " " + p.texto
			pendiente.fin = p.fin
			// Dos vacías seguidas se acumulan hasta encontrar una con peso.
			if palabrasVacias[limpia] {
				continue
			}
			out = append(out, *pendiente)
			pendiente = nil
			continue
		}

		if palabrasVacias[limpia] {
			copia := p
			pendiente = &copia
			continue
		}
		out = append(out, p)
	}

	// Si el texto acaba en palabra vacía no hay a quién pegarla; se muestra con
	// la anterior antes que descartarla.
	if pendiente != nil {
		if len(out) > 0 {
			out[len(out)-1].texto += " " + pendiente.texto
			out[len(out)-1].fin = pendiente.fin
		} else {
			out = append(out, *pendiente)
		}
	}
	return out
}

// eventosKaraoke deja la línea completa en pantalla y resalta la palabra que
// se está diciendo. Se emite un evento por palabra, cada uno con la línea
// entera y solo esa palabra coloreada.
func eventosKaraoke(b *strings.Builder, grupos []grupo, s perfil.Subtitulos) {
	activo := colorASS(s.ColorActivo)

	for _, g := range grupos {
		for i, p := range g.palabras {
			fin := p.fin
			if i+1 < len(g.palabras) {
				fin = g.palabras[i+1].inicio // sin huecos dentro de la línea
			} else {
				fin = g.fin()
			}

			partes := make([]string, len(g.palabras))
			for j, q := range g.palabras {
				if j == i {
					// \r devuelve al estilo base después de la palabra activa.
					partes[j] = fmt.Sprintf(`{\c%s\fscx112\fscy112}%s{\r}`, activo, q.texto)
				} else {
					partes[j] = q.texto
				}
			}
			tags := ""
			if i == 0 {
				tags = `\fad(70,0)` // solo la primera aparición de la línea funde
			}
			dialogo(b, p.inicio, fin, tags, strings.Join(partes, " "))
		}
	}
}

// tagsPop arma el rebote de entrada. Los tiempos de \t son milisegundos
// relativos al inicio del evento.
func tagsPop(s perfil.Subtitulos) string {
	sobre := s.EscalaPop
	if sobre <= 100 {
		sobre = 112
	}
	inicial := 60
	return fmt.Sprintf(`\fad(60,50)\fscx%d\fscy%d\t(0,110,\fscx%d\fscy%d)\t(110,190,\fscx100\fscy100)`,
		inicial, inicial, sobre, sobre)
}

func dialogo(b *strings.Builder, inicio, fin time.Duration, tags, texto string) {
	if fin <= inicio {
		fin = inicio + 300*time.Millisecond
	}
	if tags != "" {
		texto = "{" + tags + "}" + texto
	}
	fmt.Fprintf(b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
		tiempoASS(inicio), tiempoASS(fin), texto)
}

// agrupar junta palabras en líneas. Además del tope de palabras, corta cuando
// hay una pausa larga: respetar el silencio del narrador se lee mucho mejor
// que rellenar la línea hasta el límite.
func agrupar(palabras []palabra, porLinea int) []grupo {
	if porLinea <= 0 {
		porLinea = 4
	}
	const pausaLarga = 550 * time.Millisecond

	var grupos []grupo
	var actual []palabra

	cerrar := func() {
		if len(actual) > 0 {
			grupos = append(grupos, grupo{palabras: actual})
			actual = nil
		}
	}

	for i, p := range palabras {
		actual = append(actual, p)
		if len(actual) >= porLinea {
			cerrar()
			continue
		}
		if i+1 < len(palabras) && palabras[i+1].inicio-p.fin >= pausaLarga {
			cerrar()
		}
	}
	cerrar()
	return grupos
}

// colorASS normaliza el formato de color: en un archivo ASS los colores van
// como &HAABBGGRR sin el & final que sí lleva force_style.
func colorASS(c string) string {
	c = strings.TrimSpace(c)
	c = strings.TrimSuffix(c, "&")
	if c == "" {
		return "&H00FFFFFF"
	}
	if !strings.HasPrefix(c, "&H") {
		c = "&H" + strings.TrimPrefix(c, "#")
	}
	return c
}

// tiempoASS formatea como H:MM:SS.cc (centésimas), que es lo que espera ASS.
func tiempoASS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Milliseconds())
	cs := (total % 1000) / 10
	s := (total / 1000) % 60
	m := (total / 60000) % 60
	h := total / 3600000
	return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, cs)
}

func leerSRT(ruta string) ([]palabra, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var palabras []palabra
	var actual *palabra
	var texto []string

	cerrar := func() {
		if actual != nil && len(texto) > 0 {
			actual.texto = strings.Join(texto, " ")
			palabras = append(palabras, *actual)
		}
		actual, texto = nil, nil
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		// El SRT puede traer marca de orden de bytes al inicio.
		linea := strings.TrimSpace(strings.TrimPrefix(sc.Text(), string(rune(0xFEFF))))

		if linea == "" {
			cerrar()
			continue
		}
		if strings.Contains(linea, "-->") {
			ini, fin, err := parsearRango(linea)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ruta, err)
			}
			cerrar()
			actual = &palabra{inicio: ini, fin: fin}
			continue
		}
		if actual == nil {
			continue // número de secuencia: no aporta nada
		}
		texto = append(texto, escaparASS(linea))
	}
	cerrar()
	return palabras, sc.Err()
}

func parsearRango(linea string) (time.Duration, time.Duration, error) {
	partes := strings.SplitN(linea, "-->", 2)
	if len(partes) != 2 {
		return 0, 0, fmt.Errorf("rango de tiempo ilegible: %q", linea)
	}
	ini, err := parsearTiempo(partes[0])
	if err != nil {
		return 0, 0, err
	}
	campos := strings.Fields(strings.TrimSpace(partes[1]))
	if len(campos) == 0 {
		return 0, 0, fmt.Errorf("falta el tiempo final en %q", linea)
	}
	// Puede traer coordenadas de posición después del tiempo final; se ignoran.
	fin, err := parsearTiempo(campos[0])
	if err != nil {
		return 0, 0, err
	}
	return ini, fin, nil
}

// parsearTiempo acepta HH:MM:SS,mmm y HH:MM:SS.mmm
func parsearTiempo(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	campos := strings.Split(s, ":")
	if len(campos) != 3 {
		return 0, fmt.Errorf("tiempo ilegible: %q", s)
	}
	h, err1 := strconv.Atoi(campos[0])
	m, err2 := strconv.Atoi(campos[1])
	seg, err3 := strconv.ParseFloat(campos[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, fmt.Errorf("tiempo ilegible: %q", s)
	}
	return time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(seg*float64(time.Second)), nil
}

// escaparASS neutraliza los caracteres con significado propio en ASS, para que
// una transcripción con llaves no rompa el renderizado.
func escaparASS(s string) string {
	r := strings.NewReplacer("{", "(", "}", ")")
	return r.Replace(s)
}
