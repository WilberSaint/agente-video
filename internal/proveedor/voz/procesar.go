package voz

import (
	"context"
	"fmt"
	"os"
	"strings"

	"agente-video/internal/herramientas"
	"agente-video/internal/perfil"
)

// Mejorar aplica una cadena de procesado a la voz sintetizada.
//
// Un TTS crudo suena plano por tres razones concretas, y cada filtro ataca una:
//
//   - Le sobra grave y le falta presencia. El motor genera un espectro parejo,
//     sin la curva que aplicaría cualquier ingeniero a una voz hablada.
//   - La dinámica es irregular: unas sílabas salen bastante más fuertes que
//     otras, lo que obliga a subir el volumen general y delata la síntesis.
//   - Las eses salen afiladas, porque no hay un micrófono real que las suavice.
//
// Es la diferencia entre "una voz de computadora" y "una voz grabada con
// prisa", y no cuesta nada: son unos segundos de ffmpeg.
func Mejorar(ctx context.Context, entrada, salida string, v perfil.Voz) error {
	cadena := construirCadena(v)
	if cadena == "" {
		return nil
	}
	if _, err := herramientas.Correr(ctx, "ffmpeg", "-y", "-loglevel", "error",
		"-i", entrada, "-af", cadena,
		// loudnorm remuestrea a 192 kHz por dentro; sin fijar la salida el
		// archivo acaba pesando ocho veces más para el mismo audio.
		"-ar", "22050", "-c:a", "pcm_s16le", salida); err != nil {
		return fmt.Errorf("procesando la voz: %w", err)
	}
	if info, err := os.Stat(salida); err != nil || info.Size() == 0 {
		return fmt.Errorf("el procesado no produjo audio")
	}
	return nil
}

func construirCadena(v perfil.Voz) string {
	if !v.Procesar {
		return ""
	}
	presencia := v.Presencia
	if presencia == 0 {
		presencia = 3.5
	}

	filtros := []string{
		// Nada por debajo de 80 Hz aporta a una voz: solo retumbe.
		"highpass=f=80",
		// Bajar la zona "de caja" quita esa sensación de voz enlatada.
		"equalizer=f=250:t=q:w=1.2:g=-2.5",
		// Realce de presencia: es lo que más se nota en inteligibilidad,
		// sobre todo cuando encima va música.
		fmt.Sprintf("equalizer=f=3200:t=q:w=1.4:g=%.1f", presencia),
		// Domar las eses, que el sintetizador exagera.
		"deesser=i=0.35",
		// Nivelar: sin esto hay que subir el volumen general para que se
		// entiendan las sílabas flojas, y ahí se pierde la mezcla.
		"acompressor=threshold=-18dB:ratio=3:attack=15:release=180:makeup=2",
		// Volumen final coherente entre videos. -16 LUFS es lo habitual en
		// plataformas sociales.
		"loudnorm=I=-16:TP=-1.5:LRA=11",
	}
	return strings.Join(filtros, ",")
}
