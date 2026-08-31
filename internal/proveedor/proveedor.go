// Package proveedor define los contratos que separan el pipeline de la
// tecnología concreta. Cambiar de Ken Burns a Sora, o de Piper a ElevenLabs,
// es escribir un tipo nuevo que cumpla la interfaz: el pipeline no se toca.
package proveedor

import (
	"context"
	"strings"

	"agente-video/internal/perfil"
)

// ---------- Guion ----------

type Escena struct {
	N         int    `json:"n"`
	Narracion string `json:"narracion"`
	Prompt    string `json:"prompt"`
}

type GuionGenerado struct {
	Titulo      string   `json:"titulo"`
	Descripcion string   `json:"descripcion"`
	Hashtags    []string `json:"hashtags"`
	Escenas     []Escena `json:"escenas"`
}

func (g *GuionGenerado) NarracionCompleta() string {
	partes := make([]string, 0, len(g.Escenas))
	for _, e := range g.Escenas {
		if t := strings.TrimSpace(e.Narracion); t != "" {
			partes = append(partes, t)
		}
	}
	return strings.Join(partes, " ")
}

type Guionista interface {
	Nombre() string
	Generar(ctx context.Context, p *perfil.Perfil, tema string) (*GuionGenerado, error)
}

// ---------- Imagen ----------

type PeticionImagen struct {
	Prompt   string
	Negativo string
	Semilla  int64
	Ancho    int
	Alto     int
	Destino  string
}

type Imagenero interface {
	Nombre() string
	Generar(ctx context.Context, req PeticionImagen) (string, error)
}

// ---------- Voz ----------

type PeticionVoz struct {
	Texto     string
	Modelo    string
	Velocidad float64
	Destino   string // .wav
}

type Locutor interface {
	Nombre() string
	Sintetizar(ctx context.Context, req PeticionVoz) error
}

// ---------- Subtítulos ----------

type PeticionSubtitulos struct {
	Audio      string
	Idioma     string
	DestinoSRT string
}

type Subtitulador interface {
	Nombre() string
	Generar(ctx context.Context, req PeticionSubtitulos) error
}

// ---------- Video ----------

type PeticionVideo struct {
	Perfil    *perfil.Perfil
	Imagenes  []string
	Audio     string
	SRT       string
	Destino   string // .mp4
	MusicaSrc string
}

type Videasta interface {
	Nombre() string
	Ensamblar(ctx context.Context, req PeticionVideo) error
}
