// Package video ensambla el video final.
//
// KenBurns construye un solo grafo de filtros de ffmpeg: cada imagen recibe un
// zoom/paneo lento, las escenas se encadenan con transiciones cruzadas, se
// queman los subtítulos y se mezcla narración con música. Todo en una pasada.
package video

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"agente-video/internal/herramientas"
	"agente-video/internal/proveedor"
)

// factorEscala da margen para hacer zoom sin que la imagen se vea borrosa.
// Subirlo mejora la nitidez y encarece bastante el render.
const factorEscala = 1.5

type KenBurns struct {
	Preset string // preset de libx264; en 4 núcleos "veryfast" es el punto dulce
	CRF    int
	// Aviso recibe advertencias no fatales del render. Si es nil se imprimen.
	Aviso func(formato string, args ...any)
}

func (k *KenBurns) avisar(formato string, args ...any) {
	if k.Aviso != nil {
		k.Aviso(formato, args...)
		return
	}
	fmt.Printf("aviso: "+formato+"\n", args...)
}

func NuevoKenBurns() *KenBurns { return &KenBurns{Preset: "veryfast", CRF: 21} }

func (k *KenBurns) Nombre() string { return "kenburns(ffmpeg)" }

func (k *KenBurns) Ensamblar(ctx context.Context, req proveedor.PeticionVideo) error {
	p := req.Perfil
	n := len(req.Imagenes)
	if n == 0 {
		return fmt.Errorf("no hay imágenes que ensamblar")
	}

	duracionAudio, err := herramientas.DuracionSeg(ctx, req.Audio)
	if err != nil {
		return fmt.Errorf("midiendo la narración: %w", err)
	}

	trans := p.Video.TransicionSeg
	// Con n escenas hay n-1 transiciones; cada una "come" tiempo de dos escenas.
	if trans*float64(n-1) >= duracionAudio*0.5 {
		trans = duracionAudio * 0.5 / math.Max(1, float64(n-1))
	}
	// Duración por escena para que el total case exacto con el audio:
	//   n*d - (n-1)*trans = duracionAudio
	d := (duracionAudio + float64(n-1)*trans) / float64(n)

	fps := p.Formato.FPS
	anchoEsc := parEntero(int(float64(p.Formato.Ancho) * factorEscala))
	altoEsc := parEntero(int(float64(p.Formato.Alto) * factorEscala))
	frames := int(math.Round(d * float64(fps)))

	var args []string
	args = append(args, "-y", "-loglevel", "error", "-stats")

	// Una imagen fija = un frame de entrada; zoompan se encarga de estirarla.
	for _, img := range req.Imagenes {
		args = append(args, "-i", img)
	}
	idxAudio := n
	args = append(args, "-i", req.Audio)

	idxMusica := -1
	if req.MusicaSrc != "" {
		if _, err := os.Stat(req.MusicaSrc); err == nil {
			idxMusica = n + 1
			// La música se repite si es más corta que la narración.
			args = append(args, "-stream_loop", "-1", "-i", req.MusicaSrc)
		}
	}

	var filtros []string

	for i := range req.Imagenes {
		// Alternamos acercar y alejar para que no se sienta repetitivo.
		var zoom string
		if i%2 == 0 {
			zoom = fmt.Sprintf("'1+(%.4f-1)*on/%d'", p.Video.Zoom, frames)
		} else {
			zoom = fmt.Sprintf("'%.4f-(%.4f-1)*on/%d'", p.Video.Zoom, p.Video.Zoom, frames)
		}
		filtros = append(filtros, fmt.Sprintf(
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,"+
				"zoompan=z=%s:d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d:fps=%d,"+
				"setsar=1,format=yuv420p[v%d]",
			i, anchoEsc, altoEsc, anchoEsc, altoEsc,
			zoom, frames, p.Formato.Ancho, p.Formato.Alto, fps, i))
	}

	// Cadena de transiciones: el desplazamiento k-ésimo es k*(d-trans).
	ultimo := "v0"
	for i := 1; i < n; i++ {
		salida := fmt.Sprintf("x%d", i)
		offset := float64(i) * (d - trans)
		filtros = append(filtros, fmt.Sprintf(
			"[%s][v%d]xfade=transition=%s:duration=%.3f:offset=%.3f[%s]",
			ultimo, i, p.Video.Transicion, trans, offset, salida))
		ultimo = salida
	}

	if p.Subtitulos.Activos && req.SRT != "" {
		if _, err := os.Stat(req.SRT); err == nil {
			// Si la fuente no está instalada, libass no sustituye por otra:
			// deja de dibujar, sin error. Preferimos una fuente distinta a
			// quedarnos sin subtítulos.
			fuente := p.Subtitulos.Fuente
			if fuente != "" && !herramientas.FuenteInstalada(fuente) {
				k.avisar("la fuente %q no está instalada; se usará Arial. "+
					"Instálala o cambia subtitulos.fuente en el perfil.", fuente)
				fuente = "Arial"
			}
			if fuente == "" {
				fuente = "Arial"
			}

			// Generamos el ASS con la resolución real del video (ver ass.go).
			rutaASS := strings.TrimSuffix(req.SRT, filepath.Ext(req.SRT)) + ".ass"
			if err := generarASS(req.SRT, rutaASS, p, fuente); err != nil {
				return fmt.Errorf("preparando subtítulos: %w", err)
			}
			filtros = append(filtros, fmt.Sprintf("[%s]ass='%s'[vout]",
				ultimo, herramientas.RutaParaFiltro(rutaASS)))
			ultimo = "vout"
		}
	}

	etiquetaAudio := fmt.Sprintf("%d:a", idxAudio)
	if idxMusica >= 0 {
		vol := p.Video.VolumenMusica
		if vol <= 0 {
			vol = 0.12
		}
		filtros = append(filtros,
			fmt.Sprintf("[%d:a]volume=%.3f[mus]", idxMusica, vol),
			fmt.Sprintf("[%d:a][mus]amix=inputs=2:duration=first:dropout_transition=0,"+
				"afade=t=out:st=%.2f:d=1.0[aout]", idxAudio, math.Max(0, duracionAudio-1.0)))
		etiquetaAudio = "aout"
	}

	if err := os.MkdirAll(filepath.Dir(req.Destino), 0o755); err != nil {
		return err
	}

	args = append(args,
		"-filter_complex", strings.Join(filtros, ";"),
		"-map", "["+ultimo+"]",
		"-map", etiquetaSalida(etiquetaAudio),
		"-c:v", "libx264", "-preset", k.Preset, "-crf", fmt.Sprint(k.CRF),
		"-pix_fmt", "yuv420p", "-r", fmt.Sprint(fps),
		"-c:a", "aac", "-b:a", "192k",
		"-movflags", "+faststart",
		"-shortest",
		req.Destino,
	)

	if _, err := herramientas.Correr(ctx, "ffmpeg", args...); err != nil {
		return err
	}
	return nil
}

// Los mapas de ffmpeg usan corchetes para etiquetas de filtro pero no para
// flujos de entrada (0:a).
func etiquetaSalida(e string) string {
	if strings.Contains(e, ":") {
		return e
	}
	return "[" + e + "]"
}

func parEntero(v int) int {
	if v%2 != 0 {
		return v + 1
	}
	return v
}

var _ proveedor.Videasta = (*KenBurns)(nil)
