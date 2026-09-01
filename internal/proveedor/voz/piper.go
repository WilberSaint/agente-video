// Package voz implementa la síntesis de narración.
package voz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agente-video/internal/herramientas"
	"agente-video/internal/proveedor"
)

// Piper corre 100% local en CPU, es gratis y no tiene cuota.
// Binario: https://github.com/rhasspy/piper/releases
// Voces:   https://huggingface.co/rhasspy/piper-voices  (es_MX / es_ES)
type Piper struct{}

func NuevoPiper() *Piper { return &Piper{} }

func (p *Piper) Nombre() string { return "piper" }

func (p *Piper) Sintetizar(ctx context.Context, req proveedor.PeticionVoz) error {
	if req.Modelo == "" {
		return fmt.Errorf("piper necesita la ruta al modelo .onnx (voz.modelo en el perfil)")
	}
	if _, err := os.Stat(req.Modelo); err != nil {
		return fmt.Errorf("no se encontró el modelo de voz %s: %w", req.Modelo, err)
	}
	if err := os.MkdirAll(filepath.Dir(req.Destino), 0o755); err != nil {
		return err
	}

	// En Piper la escala es inversa: length_scale grande = habla más lento.
	escala := 1.0
	if req.Voz.Velocidad > 0 {
		escala = 1.0 / req.Voz.Velocidad
	}

	// Si hay procesado, Piper escribe a un archivo intermedio y la cadena de
	// filtros produce el definitivo. Así el crudo queda al lado para comparar.
	destinoPiper := req.Destino
	if req.Voz.Procesar {
		destinoPiper = strings.TrimSuffix(req.Destino, ".wav") + ".crudo.wav"
	}

	args := []string{
		"--model", req.Modelo,
		"--output_file", destinoPiper,
		"--length_scale", fmt.Sprintf("%.3f", escala),
		"--sentence_silence", "0.35",
	}
	// Los valores por defecto de Piper son los que producen esa cadencia de
	// metrónomo; subirlos rompe la regularidad y suena bastante menos sintético.
	if req.Voz.Expresividad > 0 {
		args = append(args, "--noise_w", fmt.Sprintf("%.3f", req.Voz.Expresividad))
	}
	if req.Voz.Variacion > 0 {
		args = append(args, "--noise_scale", fmt.Sprintf("%.3f", req.Voz.Variacion))
	}

	if _, err := herramientas.CorrerConEntrada(ctx, "piper", req.Texto, args...); err != nil {
		return err
	}
	if info, err := os.Stat(destinoPiper); err != nil || info.Size() == 0 {
		return fmt.Errorf("piper no produjo audio en %s", destinoPiper)
	}

	if req.Voz.Procesar {
		if err := Mejorar(ctx, destinoPiper, req.Destino, req.Voz); err != nil {
			return err
		}
	}
	return nil
}

var _ proveedor.Locutor = (*Piper)(nil)
