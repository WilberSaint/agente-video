// Package voz implementa la síntesis de narración.
package voz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	if req.Velocidad > 0 {
		escala = 1.0 / req.Velocidad
	}

	_, err := herramientas.CorrerConEntrada(ctx, "piper", req.Texto,
		"--model", req.Modelo,
		"--output_file", req.Destino,
		"--length_scale", fmt.Sprintf("%.3f", escala),
		"--sentence_silence", "0.35",
	)
	if err != nil {
		return err
	}
	if info, err := os.Stat(req.Destino); err != nil || info.Size() == 0 {
		return fmt.Errorf("piper no produjo audio en %s", req.Destino)
	}
	return nil
}

var _ proveedor.Locutor = (*Piper)(nil)
