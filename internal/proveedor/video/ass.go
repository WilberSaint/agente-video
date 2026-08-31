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

// Este archivo existe por una razón concreta.
//
// El filtro subtitles= de ffmpeg convierte el SRT a ASS usando un lienzo fijo
// de 384x288, y force_style se interpreta en ESE espacio, no en píxeles del
// video. Con un video de 1080x1920, un MarginV=300 empuja el texto fuera de un
// lienzo de 288 de alto: los subtítulos desaparecen, sin error y con ffmpeg
// saliendo en 0.
//
// Generamos entonces el ASS nosotros, con PlayResX/PlayResY iguales al video.
// Así cada valor del perfil está en píxeles reales y lo que se configura es lo
// que se ve.

type lineaSubtitulo struct {
	inicio time.Duration
	fin    time.Duration
	texto  string
}

// generarASS convierte el SRT de whisper en un ASS con el estilo del perfil.
func generarASS(rutaSRT, destino string, p *perfil.Perfil, fuente string) error {
	lineas, err := leerSRT(rutaSRT)
	if err != nil {
		return err
	}
	if len(lineas) == 0 {
		return fmt.Errorf("%s no contiene subtítulos", rutaSRT)
	}

	negrita := 0
	if p.Subtitulos.TamPx > 0 {
		negrita = -1 // en ASS, -1 es verdadero
	}
	// Márgenes laterales generosos: en vertical el texto no debe tocar el borde.
	margenLateral := p.Formato.Ancho / 12

	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\n")
	fmt.Fprintf(&b, "; generado por agente-video\n")
	fmt.Fprintf(&b, "ScriptType: v4.00+\n")
	fmt.Fprintf(&b, "PlayResX: %d\n", p.Formato.Ancho)
	fmt.Fprintf(&b, "PlayResY: %d\n", p.Formato.Alto)
	fmt.Fprintf(&b, "ScaledBorderAndShadow: yes\n")
	fmt.Fprintf(&b, "WrapStyle: 0\n\n")

	fmt.Fprintf(&b, "[V4+ Styles]\n")
	fmt.Fprintf(&b, "Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, "+
		"OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, "+
		"Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, "+
		"MarginV, Encoding\n")
	fmt.Fprintf(&b, "Style: Default,%s,%d,%s,%s,%s,&H00000000,%d,0,0,0,100,100,0,0,1,%d,0,2,%d,%d,%d,1\n\n",
		fuente,
		p.Subtitulos.TamPx,
		colorASS(p.Subtitulos.ColorPrimario),
		colorASS(p.Subtitulos.ColorPrimario),
		colorASS(p.Subtitulos.ColorBorde),
		negrita,
		p.Subtitulos.GrosorBorde,
		margenLateral, margenLateral,
		p.Subtitulos.MargenV)

	fmt.Fprintf(&b, "[Events]\n")
	fmt.Fprintf(&b, "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, l := range lineas {
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			tiempoASS(l.inicio), tiempoASS(l.fin), l.texto)
	}

	return os.WriteFile(destino, []byte(b.String()), 0o644)
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

func leerSRT(ruta string) ([]lineaSubtitulo, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lineas []lineaSubtitulo
	var actual *lineaSubtitulo
	var texto []string

	cerrar := func() {
		if actual != nil && len(texto) > 0 {
			// \N es el salto de línea de ASS.
			actual.texto = strings.Join(texto, `\N`)
			lineas = append(lineas, *actual)
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
			actual = &lineaSubtitulo{inicio: ini, fin: fin}
			continue
		}
		if actual == nil {
			continue // número de secuencia: no aporta nada
		}
		texto = append(texto, escaparASS(linea))
	}
	cerrar()
	return lineas, sc.Err()
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
	// Puede traer coordenadas de posición después del tiempo final; se ignoran.
	fin, err := parsearTiempo(strings.Fields(strings.TrimSpace(partes[1]))[0])
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
	r := strings.NewReplacer("{", "\\{", "}", "\\}")
	return r.Replace(s)
}
