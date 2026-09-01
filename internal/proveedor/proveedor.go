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

// Plano es una imagen concreta con su encuadre. Una escena tiene entre uno y
// tres: el video se construye con imágenes fijas, así que la sensación de
// movimiento viene de cambiar de plano dentro de la misma idea, no de que algo
// se mueva dentro de la imagen.
type Plano struct {
	Prompt   string `json:"prompt"`
	Encuadre string `json:"encuadre"` // general | medio | cercano | detalle | cenital

	// Sujeto identifica al personaje o lugar recurrente que aparece en este
	// plano. Todos los planos que compartan sujeto se generan con la misma
	// semilla, y eso mejora mucho que se vean como el mismo personaje: medido
	// sobre un caso real, con semillas distintas el abrigo cambiaba de color y
	// la cara salía deformada; con semilla compartida, no.
	Sujeto string `json:"sujeto,omitempty"`
}

// SemillaDe devuelve la semilla que le toca a un plano. Los planos de un mismo
// sujeto comparten semilla; el resto varía para no repetir composiciones.
func (p Plano) SemillaDe(base int64, escena, indice int) int64 {
	if s := strings.TrimSpace(strings.ToLower(p.Sujeto)); s != "" {
		return base + int64(hashFNV(s)%100000)
	}
	return base + int64(escena*10+indice)
}

// hashFNV es FNV-1a de 32 bits: estable entre ejecuciones y entre máquinas, que
// es lo único que se le pide. No se usa para nada criptográfico.
func hashFNV(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

type Escena struct {
	N         int     `json:"n"`
	Narracion string  `json:"narracion"`
	Planos    []Plano `json:"planos"`

	// Prompt es el formato anterior, de una sola imagen por escena. Se conserva
	// para que los checkpoints ya generados sigan sirviendo.
	Prompt string `json:"prompt,omitempty"`
}

// Normalizar convierte el formato antiguo al nuevo y numera las escenas.
func (g *GuionGenerado) Normalizar() {
	for i := range g.Escenas {
		e := &g.Escenas[i]
		e.N = i + 1
		if len(e.Planos) == 0 && e.Prompt != "" {
			e.Planos = []Plano{{Prompt: e.Prompt, Encuadre: "medio"}}
		}
	}
}

// TotalPlanos es cuántas imágenes hay que generar en total.
func (g *GuionGenerado) TotalPlanos() int {
	n := 0
	for _, e := range g.Escenas {
		n += len(e.Planos)
	}
	return n
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

// EscenaRender es una escena ya con sus imágenes en disco. El ensamblador
// necesita la narración además de las rutas: con ella y los tiempos por palabra
// del SRT calcula cuándo empieza y termina cada escena, para que las imágenes
// cambien al ritmo del relato y no a intervalos fijos.
type EscenaRender struct {
	Narracion string
	Imagenes  []string
	Encuadres []string
}

type PeticionVideo struct {
	Perfil    *perfil.Perfil
	Escenas   []EscenaRender
	Audio     string
	SRT       string
	Destino   string // .mp4
	MusicaSrc string
}

// Imagenes aplana todas las rutas en el orden en que aparecen en el video.
func (p PeticionVideo) Imagenes() []string {
	var rutas []string
	for _, e := range p.Escenas {
		rutas = append(rutas, e.Imagenes...)
	}
	return rutas
}

type Videasta interface {
	Nombre() string
	Ensamblar(ctx context.Context, req PeticionVideo) error
}
