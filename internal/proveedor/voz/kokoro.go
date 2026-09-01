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

// Kokoro es la voz local de mejor calidad.
//
// Corre en CPU como Piper, y también es gratis e ilimitada, pero su
// arquitectura es más moderna y se nota: menos cadencia de metrónomo y una
// entonación bastante más natural. La contrapartida es que necesita Python,
// que es la razón por la que no venía de entrada.
//
// Va más lento que Piper —unas dos veces el tiempo real frente a diez— pero en
// un video de doce minutos, veinte segundos más de síntesis no cambian nada.
type Kokoro struct {
	python   string
	guion    string
	modelo   string
	vocesBin string
}

func NuevoKokoro(python, guion, modelo, vocesBin string) *Kokoro {
	if python == "" {
		python = filepath.Join("bin", "python", "python.exe")
	}
	if guion == "" {
		guion = filepath.Join("bin", "kokoro", "hablar.py")
	}
	if modelo == "" {
		modelo = filepath.Join("bin", "kokoro", "kokoro-v1.0.onnx")
	}
	if vocesBin == "" {
		vocesBin = filepath.Join("bin", "kokoro", "voices-v1.0.bin")
	}
	return &Kokoro{python: python, guion: guion, modelo: modelo, vocesBin: vocesBin}
}

func (k *Kokoro) Nombre() string { return "kokoro" }

// VocesEspanol son las que trae el modelo v1.0. El prefijo indica idioma y sexo:
// ef = español femenino, em = español masculino.
var VocesEspanol = []string{"ef_dora", "em_alex", "em_santa"}

func (k *Kokoro) Sintetizar(ctx context.Context, req proveedor.PeticionVoz) error {
	// Los tres archivos se comprueban por separado para poder decir cuál falta:
	// "kokoro no está instalado" no ayuda a nadie a arreglarlo.
	for _, f := range []struct{ ruta, que string }{
		{k.python, "el intérprete de Python (ejecuta .\\instalar-python.ps1)"},
		{k.guion, "el puente hablar.py"},
		{k.modelo, "el modelo kokoro-v1.0.onnx (ejecuta .\\instalar-kokoro.ps1)"},
		{k.vocesBin, "el banco de voces voices-v1.0.bin"},
	} {
		if _, err := os.Stat(f.ruta); err != nil {
			return proveedor.Permanente(
				fmt.Errorf("falta %s en %s", f.que, f.ruta))
		}
	}

	voz := req.Voz.Modelo
	// En Piper el modelo es una ruta a un .onnx; aquí es el nombre de una voz.
	// Un perfil que venía de Piper traería una ruta, que Kokoro no entiende.
	if voz == "" || strings.ContainsAny(voz, "/\\") {
		voz = "em_alex"
	}

	if err := os.MkdirAll(filepath.Dir(req.Destino), 0o755); err != nil {
		return err
	}

	destino := req.Destino
	if req.Voz.Procesar {
		destino = strings.TrimSuffix(req.Destino, ".wav") + ".crudo.wav"
	}

	velocidad := req.Voz.Velocidad
	if velocidad <= 0 {
		velocidad = 1.0
	}

	_, err := herramientas.CorrerConEntrada(ctx, k.python, req.Texto,
		k.guion,
		"--modelo", k.modelo,
		"--voces", k.vocesBin,
		"--voz", voz,
		"--idioma", "es",
		"--velocidad", fmt.Sprintf("%.2f", velocidad),
		"--salida", destino,
	)
	if err != nil {
		return err
	}
	if info, err := os.Stat(destino); err != nil || info.Size() == 0 {
		return fmt.Errorf("kokoro no produjo audio en %s", destino)
	}

	if req.Voz.Procesar {
		return Mejorar(ctx, destino, req.Destino, req.Voz)
	}
	return nil
}

var _ proveedor.Locutor = (*Kokoro)(nil)
