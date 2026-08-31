package video

import (
	"fmt"
	"os"
	"time"

	"agente-video/internal/perfil"
	"agente-video/internal/proveedor"
)

// Mezcla del audio: narración, música y efectos en los cortes.
//
// Los efectos importan más de lo que parece en un video de imágenes fijas.
// Cuando la imagen cambia y no se oye nada, el corte se lee como un salto; con
// un barrido corto encima se lee como una decisión de edición. Es la misma
// imagen y el mismo corte: cambia solo que el oído lo acompaña.

type mezcla struct {
	filtros  []string // filtros a añadir al grafo
	etiqueta string   // etiqueta de salida del audio ya mezclado
	entradas []string // rutas nuevas que hay que añadir como -i, en orden
	efectos  int
}

// construirAudio arma la cadena de audio. idxAudio es el índice de entrada de
// la narración; las entradas que devuelve se añaden después de esa.
func construirAudio(p *perfil.Perfil, req proveedor.PeticionVideo, tramos []tramo,
	idxAudio int, duracion float64, avisar func(string, ...any)) mezcla {

	m := mezcla{}
	siguiente := idxAudio + 1

	// La narración manda: todo lo demás se mezcla por debajo.
	m.filtros = append(m.filtros, fmt.Sprintf(
		"[%d:a]aformat=sample_rates=44100:channel_layouts=stereo[narr]", idxAudio))
	partes := []string{"[narr]"}

	// --- música ---
	if ruta := req.MusicaSrc; ruta != "" {
		if _, err := os.Stat(ruta); err == nil {
			vol := p.Video.VolumenMusica
			if vol <= 0 {
				vol = 0.12
			}
			idx := siguiente
			siguiente++
			// -stream_loop la repite si es más corta que la narración.
			m.entradas = append(m.entradas, "-stream_loop", "-1", "-i", ruta)
			m.filtros = append(m.filtros, fmt.Sprintf(
				"[%d:a]aformat=sample_rates=44100:channel_layouts=stereo,volume=%.3f[mus]",
				idx, vol))
			partes = append(partes, "[mus]")
		} else {
			avisar("no se encontró la música %s; se omite", ruta)
		}
	}

	// --- efectos en los cortes ---
	instantes := instantesDeEfecto(req.Escenas, tramos, p.Video.EfectosEn)
	if ruta := p.RutaRelativa(p.Video.EfectoTransicion); ruta != "" && len(instantes) > 0 {
		if _, err := os.Stat(ruta); err == nil {
			idx := siguiente
			siguiente++
			m.entradas = append(m.entradas, "-i", ruta)

			vol := p.Video.VolumenEfectos
			if vol <= 0 {
				vol = 0.35
			}

			// Una sola entrada no puede usarse en varios sitios del grafo: hay
			// que dividirla en tantas copias como veces suene.
			etiquetas := make([]string, len(instantes))
			for i := range instantes {
				etiquetas[i] = fmt.Sprintf("[e%d]", i)
			}
			m.filtros = append(m.filtros, fmt.Sprintf("[%d:a]asplit=%d%s",
				idx, len(instantes), join(etiquetas)))

			for i, t := range instantes {
				ms := t.Milliseconds()
				m.filtros = append(m.filtros, fmt.Sprintf(
					"[e%d]adelay=%d|%d,aformat=sample_rates=44100:channel_layouts=stereo,"+
						"volume=%.3f[s%d]", i, ms, ms, vol, i))
				partes = append(partes, fmt.Sprintf("[s%d]", i))
			}
			m.efectos = len(instantes)
		} else {
			avisar("no se encontró el efecto %s; se omite", ruta)
		}
	}

	if len(partes) == 1 {
		// Solo narración: no hace falta mezclar nada.
		m.etiqueta = "narr"
		return m
	}

	// normalize=0 evita que amix baje el volumen de la narración cada vez que
	// se suma una pista; a cambio hay que limitar para no saturar al sumar.
	m.filtros = append(m.filtros, fmt.Sprintf(
		"%samix=inputs=%d:normalize=0:dropout_transition=0,alimiter=limit=0.95,"+
			"afade=t=out:st=%.2f:d=1.0[aout]",
		joinPartes(partes), len(partes), maxF(0, duracion-1.0)))
	m.etiqueta = "aout"
	return m
}

// instantesDeEfecto devuelve en qué momentos debe sonar el efecto.
//
// El corte inicial no lleva efecto: sonaría antes de que el espectador entienda
// qué está viendo.
func instantesDeEfecto(escenas []proveedor.EscenaRender, tramos []tramo, modo string) []time.Duration {
	if modo == "ninguno" || len(tramos) < 2 {
		return nil
	}

	if modo == "plano" {
		out := make([]time.Duration, 0, len(tramos)-1)
		for _, t := range tramos[1:] {
			out = append(out, t.inicio)
		}
		return out
	}

	// Modo "escena": solo donde empieza una idea nueva. Dentro de una escena
	// los planos siguen hablando de lo mismo, y marcarlos todos cansa.
	var out []time.Duration
	idx := 0
	for i, e := range escenas {
		if i > 0 && idx < len(tramos) {
			out = append(out, tramos[idx].inicio)
		}
		idx += len(e.Imagenes)
	}
	return out
}

func join(etiquetas []string) string {
	s := ""
	for _, e := range etiquetas {
		s += e
	}
	return s
}

func joinPartes(partes []string) string { return join(partes) }

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
