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
	"time"

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
	imagenes := req.Imagenes()
	encuadres := aplanarEncuadres(req.Escenas)
	n := len(imagenes)
	if n == 0 {
		return fmt.Errorf("no hay imágenes que ensamblar")
	}

	duracionAudio, err := herramientas.DuracionSeg(ctx, req.Audio)
	if err != nil {
		return fmt.Errorf("midiendo la narración: %w", err)
	}
	total := time.Duration(duracionAudio * float64(time.Second))

	trans := p.Video.TransicionSeg
	// Con n imágenes hay n-1 transiciones; cada una "come" tiempo de dos.
	if trans*float64(n-1) >= duracionAudio*0.5 {
		trans = duracionAudio * 0.5 / math.Max(1, float64(n-1))
	}

	// Cada imagen dura lo que dura su parte del relato. Sin los tiempos por
	// palabra no hay forma de saberlo y se cae al reparto uniforme.
	var palabras []palabra
	if req.SRT != "" {
		if ps, err := leerSRT(req.SRT); err == nil {
			palabras = ps
		} else {
			k.avisar("no se pudieron leer los tiempos de %s (%v); las imágenes "+
				"cambiarán a intervalos iguales", req.SRT, err)
		}
	}
	tramos := repartir(req.Escenas, palabras, total,
		p.Video.MinSegPorImagen, p.Video.MaxSegPorImagen)

	fps := p.Formato.FPS
	anchoEsc := parEntero(int(float64(p.Formato.Ancho) * factorEscala))
	altoEsc := parEntero(int(float64(p.Formato.Alto) * factorEscala))

	var args []string
	args = append(args, "-y", "-loglevel", "error", "-stats")

	// Una imagen fija = un frame de entrada; zoompan se encarga de estirarla.
	for _, img := range imagenes {
		args = append(args, "-i", img)
	}
	idxAudio := n
	args = append(args, "-i", req.Audio)

	// La mezcla decide qué entradas de audio más hacen falta (música, efectos)
	// y con qué filtros se combinan.
	mez := construirAudio(p, req, tramos, idxAudio, duracionAudio, k.avisar)
	args = append(args, mez.entradas...)

	var filtros []string

	// Cada imagen dura lo suyo, así que su zoompan lleva su propio número de
	// frames. Se le suma la transición porque durante el cruce se ven las dos.
	duraciones := make([]float64, n)
	for i := range imagenes {
		d := tramos[i].dura().Seconds()
		if i < n-1 {
			d += trans
		}
		duraciones[i] = d

		frames := int(math.Round(d * float64(fps)))
		if frames < 1 {
			frames = 1
		}

		zoomMax := zoomDeEncuadre(p.Video.Zoom, encuadres[i])
		// Alternamos acercar y alejar para que no se sienta repetitivo.
		var zoom string
		if i%2 == 0 {
			zoom = fmt.Sprintf("'1+(%.4f-1)*on/%d'", zoomMax, frames)
		} else {
			zoom = fmt.Sprintf("'%.4f-(%.4f-1)*on/%d'", zoomMax, zoomMax, frames)
		}
		filtros = append(filtros, fmt.Sprintf(
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,"+
				"zoompan=z=%s:d=%d:x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':s=%dx%d:fps=%d,"+
				"setsar=1,format=yuv420p[v%d]",
			i, anchoEsc, altoEsc, anchoEsc, altoEsc,
			zoom, frames, p.Formato.Ancho, p.Formato.Alto, fps, i))
	}

	// Cadena de transiciones. El desplazamiento de cada cruce es el instante en
	// que termina la imagen anterior, que ya no es un múltiplo fijo.
	ultimo := "v0"
	var acumulado float64
	for i := 1; i < n; i++ {
		salida := fmt.Sprintf("x%d", i)
		acumulado += duraciones[i-1] - trans
		filtros = append(filtros, fmt.Sprintf(
			"[%s][v%d]xfade=transition=%s:duration=%.3f:offset=%.3f[%s]",
			ultimo, i, p.Video.Transicion, trans, acumulado, salida))
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

	filtros = append(filtros, mez.filtros...)
	if mez.efectos > 0 {
		k.avisar("%d efecto(s) de sonido en los cortes", mez.efectos)
	}

	if err := os.MkdirAll(filepath.Dir(req.Destino), 0o755); err != nil {
		return err
	}

	args = append(args,
		"-filter_complex", strings.Join(filtros, ";"),
		"-map", "["+ultimo+"]",
		"-map", "["+mez.etiqueta+"]",
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

func parEntero(v int) int {
	if v%2 != 0 {
		return v + 1
	}
	return v
}

var _ proveedor.Videasta = (*KenBurns)(nil)

// aplanarEncuadres devuelve el encuadre de cada imagen en el orden del video,
// rellenando con "medio" cuando el guion no lo especificó.
func aplanarEncuadres(escenas []proveedor.EscenaRender) []string {
	var out []string
	for _, e := range escenas {
		for i := range e.Imagenes {
			if i < len(e.Encuadres) && e.Encuadres[i] != "" {
				out = append(out, e.Encuadres[i])
			} else {
				out = append(out, "medio")
			}
		}
	}
	return out
}
