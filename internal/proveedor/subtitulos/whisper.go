// Package subtitulos genera el .srt a partir del audio narrado.
package subtitulos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agente-video/internal/herramientas"
	"agente-video/internal/proveedor"
)

// Whisper usa whisper.cpp, que corre en CPU y aprovecha AVX2.
// Binario y modelos: https://github.com/ggml-org/whisper.cpp
type Whisper struct {
	modelo string // ruta al .bin (ggml-base.bin, ggml-small.bin, ...)
}

func NuevoWhisper(modelo string) *Whisper {
	if modelo == "" {
		modelo = filepath.Join("bin", "modelos", "ggml-base.bin")
	}
	return &Whisper{modelo: modelo}
}

func (w *Whisper) Nombre() string { return "whisper.cpp:" + filepath.Base(w.modelo) }

func (w *Whisper) Generar(ctx context.Context, req proveedor.PeticionSubtitulos) error {
	if _, err := os.Stat(w.modelo); err != nil {
		return fmt.Errorf("no se encontró el modelo de whisper %s: %w", w.modelo, err)
	}

	// whisper.cpp exige WAV PCM 16 kHz mono; Piper entrega 22050 Hz.
	wav16 := strings.TrimSuffix(req.DestinoSRT, ".srt") + ".16k.wav"
	if _, err := herramientas.Correr(ctx, "ffmpeg", "-y", "-loglevel", "error",
		"-i", req.Audio, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav16); err != nil {
		return fmt.Errorf("convirtiendo audio para whisper: %w", err)
	}
	defer os.Remove(wav16)

	// -of recibe el prefijo: whisper.cpp le añade la extensión .srt
	prefijo := strings.TrimSuffix(req.DestinoSRT, ".srt")
	idioma := req.Idioma
	if idioma == "" {
		idioma = "auto"
	}

	binario := binarioDisponible()
	if _, err := herramientas.Correr(ctx, binario,
		"-m", w.modelo,
		"-f", wav16,
		"-l", idioma,
		"-osrt",
		"-of", prefijo,
		"-ml", "24", // líneas cortas: legibles en vertical
		"-sow",      // partir en palabra, no a media palabra
	); err != nil {
		return err
	}

	if _, err := os.Stat(req.DestinoSRT); err != nil {
		return fmt.Errorf("whisper no generó %s", req.DestinoSRT)
	}
	return nil
}

// El ejecutable cambió de nombre entre versiones de whisper.cpp.
func binarioDisponible() string {
	for _, n := range []string{"whisper-cli", "main", "whisper"} {
		if _, err := herramientas.Buscar(n); err == nil {
			return n
		}
	}
	return "whisper-cli"
}

var _ proveedor.Subtitulador = (*Whisper)(nil)
