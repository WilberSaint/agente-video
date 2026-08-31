// Package perfil carga la configuración de cada "persona" o canal.
// Un perfil = una carpeta con su perfil.json, sus imágenes de referencia
// y su propio estilo, voz y formato. El agente es genérico; el perfil manda.
package perfil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Formato struct {
	Ancho int `json:"ancho"`
	Alto  int `json:"alto"`
	FPS   int `json:"fps"`
}

type Guion struct {
	Tono        string `json:"tono"`
	DuracionSeg int    `json:"duracion_seg"`
	Escenas     int    `json:"escenas"`
	Extra       string `json:"instrucciones_extra"`
}

type Imagen struct {
	Proveedor  string `json:"proveedor"`  // pollinations | cloudflare
	Modelo     string `json:"modelo"`
	Estilo     string `json:"estilo"`     // se añade a TODOS los prompts
	Negativo   string `json:"negativo"`
	Semilla    int64  `json:"semilla"`    // fija = mayor consistencia entre escenas
	Referencia string `json:"referencia"` // ruta relativa dentro del perfil
	Personaje  string `json:"personaje"`  // descripción física repetida en cada prompt
}

type Voz struct {
	Proveedor string  `json:"proveedor"` // piper
	Modelo    string  `json:"modelo"`    // ruta al .onnx
	Velocidad float64 `json:"velocidad"`
}

type Subtitulos struct {
	Activos       bool   `json:"activos"`
	Fuente        string `json:"fuente"`
	TamPx         int    `json:"tam_px"`
	ColorPrimario string `json:"color_primario"` // &HBBGGRR& (formato ASS)
	ColorBorde    string `json:"color_borde"`
	GrosorBorde   int    `json:"grosor_borde"`
	MargenV       int    `json:"margen_v"`
}

type Video struct {
	Proveedor     string  `json:"proveedor"` // kenburns  (futuro: sora | kling | runway)
	Transicion    string  `json:"transicion"`
	TransicionSeg float64 `json:"transicion_seg"`
	Zoom          float64 `json:"zoom"`
	Musica        string  `json:"musica"`
	VolumenMusica float64 `json:"volumen_musica"`
}

type Perfil struct {
	ID         string     `json:"id"`
	Nombre     string     `json:"nombre"`
	Idioma     string     `json:"idioma"`
	Formato    Formato    `json:"formato"`
	Guion      Guion      `json:"guion"`
	Imagen     Imagen     `json:"imagen"`
	Voz        Voz        `json:"voz"`
	Subtitulos Subtitulos `json:"subtitulos"`
	Video      Video      `json:"video"`

	Raiz string `json:"-"` // carpeta del perfil, se llena al cargar
}

// Cargar lee perfiles/<id>/perfil.json
func Cargar(dirPerfiles, id string) (*Perfil, error) {
	raiz := filepath.Join(dirPerfiles, id)
	ruta := filepath.Join(raiz, "perfil.json")
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", ruta, err)
	}
	var p Perfil
	dec := json.NewDecoder(strings.NewReader(string(datos)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("perfil.json inválido en %s: %w", ruta, err)
	}
	p.Raiz = raiz
	if p.ID == "" {
		p.ID = id
	}
	p.aplicarValoresPorDefecto()
	return &p, p.Validar()
}

// Listar devuelve los ids de todos los perfiles disponibles.
func Listar(dirPerfiles string) ([]string, error) {
	entradas, err := os.ReadDir(dirPerfiles)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entradas {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dirPerfiles, e.Name(), "perfil.json")); err == nil {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

func (p *Perfil) aplicarValoresPorDefecto() {
	if p.Formato.Ancho == 0 {
		p.Formato.Ancho = 1080
	}
	if p.Formato.Alto == 0 {
		p.Formato.Alto = 1920
	}
	if p.Formato.FPS == 0 {
		p.Formato.FPS = 30
	}
	if p.Idioma == "" {
		p.Idioma = "es"
	}
	if p.Guion.Escenas == 0 {
		p.Guion.Escenas = 8
	}
	if p.Guion.DuracionSeg == 0 {
		p.Guion.DuracionSeg = 60
	}
	if p.Voz.Velocidad == 0 {
		p.Voz.Velocidad = 1.0
	}
	if p.Video.Proveedor == "" {
		p.Video.Proveedor = "kenburns"
	}
	if p.Video.Transicion == "" {
		p.Video.Transicion = "fade"
	}
	if p.Video.TransicionSeg == 0 {
		p.Video.TransicionSeg = 0.6
	}
	if p.Video.Zoom == 0 {
		p.Video.Zoom = 1.20
	}
	if p.Imagen.Proveedor == "" {
		p.Imagen.Proveedor = "pollinations"
	}
	if p.Subtitulos.TamPx == 0 {
		p.Subtitulos.TamPx = 78
	}
	if p.Subtitulos.Fuente == "" {
		p.Subtitulos.Fuente = "Arial"
	}
	if p.Subtitulos.ColorPrimario == "" {
		p.Subtitulos.ColorPrimario = "&H00FFFFFF&"
	}
	if p.Subtitulos.ColorBorde == "" {
		p.Subtitulos.ColorBorde = "&H00000000&"
	}
	if p.Subtitulos.GrosorBorde == 0 {
		p.Subtitulos.GrosorBorde = 4
	}
	if p.Subtitulos.MargenV == 0 {
		p.Subtitulos.MargenV = 260
	}
}

func (p *Perfil) Validar() error {
	if p.ID == "" {
		return fmt.Errorf("el perfil necesita un campo \"id\"")
	}
	if p.Guion.Escenas < 2 {
		return fmt.Errorf("perfil %s: se requieren al menos 2 escenas", p.ID)
	}
	if p.Voz.Proveedor == "piper" && p.Voz.Modelo == "" {
		return fmt.Errorf("perfil %s: voz.proveedor=piper requiere voz.modelo (ruta al .onnx)", p.ID)
	}
	return nil
}

// RutaRelativa resuelve una ruta declarada en el perfil contra su carpeta.
// Las rutas absolutas se respetan tal cual.
func (p *Perfil) RutaRelativa(r string) string {
	if r == "" {
		return ""
	}
	if filepath.IsAbs(r) {
		return r
	}
	return filepath.Join(p.Raiz, r)
}
