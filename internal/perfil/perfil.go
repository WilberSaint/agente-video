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
	Proveedor  string `json:"proveedor"` // pollinations | cloudflare | local
	Modelo     string `json:"modelo"`
	Estilo     string `json:"estilo"` // se añade a TODOS los prompts
	Negativo   string `json:"negativo"`
	Semilla    int64  `json:"semilla"`    // fija = mayor consistencia entre escenas
	Referencia string `json:"referencia"` // ruta relativa dentro del perfil
	Personaje  string `json:"personaje"`  // descripción física repetida en cada prompt

	// Solo para proveedor "local": la GPU propia. Van en el perfil y no en
	// variables de entorno porque son decisiones de estilo —cuánto insistir en
	// el prompt, cuántos pasos— y cada canal puede querer las suyas.
	Servidor string  `json:"servidor,omitempty"` // http://127.0.0.1:7860
	Pasos    int     `json:"pasos,omitempty"`
	CFG      float64 `json:"cfg,omitempty"`
	Sampler  string  `json:"sampler,omitempty"`
}

type Voz struct {
	Proveedor string  `json:"proveedor"` // piper
	Modelo    string  `json:"modelo"`    // ruta al .onnx
	Velocidad float64 `json:"velocidad"`

	// Expresividad varía la duración de cada fonema. Es el parámetro que más
	// combate la sensación robótica: con todos los fonemas midiendo casi lo
	// mismo, la voz cae en una cadencia de metrónomo. 0.8 es el valor neutro
	// de Piper; subirlo a 0.9-1.0 da un habla menos regular y más humana, y
	// pasarse la vuelve inestable.
	Expresividad float64 `json:"expresividad"`
	// Variacion afecta al timbre. Por encima de 0.8 empieza a sonar ebrio.
	Variacion float64 `json:"variacion"`

	// Procesar aplica ecualización, compresión y normalización sobre el audio
	// generado. Es lo que más acerca el resultado a una voz grabada.
	Procesar  bool    `json:"procesar"`
	Presencia float64 `json:"presencia"` // dB de realce en 3.2 kHz

	// LimiteCaracteres es la cuota mensual del proveedor de pago. Al agotarse
	// se pasa solo al de respaldo en vez de dejar el lote a medias: un video
	// con peor voz es mejor que diez videos sin generar.
	LimiteCaracteres int `json:"limite_caracteres"`
	// Respaldo es el proveedor al que caer. Vacío = piper.
	Respaldo       string `json:"respaldo"`
	ModeloRespaldo string `json:"modelo_respaldo"`
}

type Subtitulos struct {
	Activos       bool   `json:"activos"`
	Fuente        string `json:"fuente"`
	TamPx         int    `json:"tam_px"`
	ColorPrimario string `json:"color_primario"` // &HBBGGRR& (formato ASS)
	ColorBorde    string `json:"color_borde"`
	GrosorBorde   int    `json:"grosor_borde"`
	MargenV       int    `json:"margen_v"`

	// Animacion: ninguna | pop | karaoke | palabra
	//   pop     – la línea entra con un rebote de escala
	//   karaoke – la línea se queda y se resalta la palabra que se dice
	//   palabra – una sola palabra a la vez, con rebote
	Animacion        string `json:"animacion"`
	PalabrasPorLinea int    `json:"palabras_por_linea"`
	ColorActivo      string `json:"color_activo"` // palabra resaltada en karaoke
	EscalaPop        int    `json:"escala_pop"`   // % de sobre-escala del rebote
}

// AnimacionesValidas son los modos que entiende el generador de subtítulos.
var AnimacionesValidas = []string{"ninguna", "pop", "karaoke", "palabra"}

type Video struct {
	Proveedor     string  `json:"proveedor"` // kenburns  (futuro: sora | kling | runway)
	Transicion    string  `json:"transicion"`
	TransicionSeg float64 `json:"transicion_seg"`
	Zoom          float64 `json:"zoom"`
	Musica        string  `json:"musica"`
	VolumenMusica float64 `json:"volumen_musica"`

	// Cotas de cuánto puede durar una imagen en pantalla. El video se hace con
	// imágenes fijas: pasado cierto punto la imagen deja de leerse como una
	// pausa intencionada y se lee como que el video se congeló.
	MinSegPorImagen float64 `json:"min_seg_por_imagen"`
	MaxSegPorImagen float64 `json:"max_seg_por_imagen"`

	// EfectoTransicion es un .wav que suena en cada corte. Sin él los cambios
	// de imagen se ven pero no se oyen, y el video se siente plano aunque la
	// edición visual sea correcta.
	EfectoTransicion string  `json:"efecto_transicion"`
	VolumenEfectos   float64 `json:"volumen_efectos"`
	// EfectosEn: escena | plano | ninguno.
	//   escena – solo al cambiar de idea. Es lo que se oye intencionado.
	//   plano  – en cada imagen. Más agresivo, propio de video muy acelerado.
	EfectosEn string `json:"efectos_en"`
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
	Personaje  Personaje  `json:"personaje"`
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
	if p.Video.MinSegPorImagen == 0 {
		p.Video.MinSegPorImagen = 1.8
	}
	if p.Video.MaxSegPorImagen == 0 {
		p.Video.MaxSegPorImagen = 5.0
	}
	if p.Personaje.Animacion == "" {
		p.Personaje.Animacion = "hablar"
	}
	if p.Video.EfectosEn == "" {
		p.Video.EfectosEn = "escena"
	}
	if p.Video.VolumenEfectos == 0 {
		p.Video.VolumenEfectos = 0.35
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
	if p.Subtitulos.Animacion == "" {
		p.Subtitulos.Animacion = "pop"
	}
	if p.Subtitulos.PalabrasPorLinea == 0 {
		p.Subtitulos.PalabrasPorLinea = 4
	}
	if p.Subtitulos.ColorActivo == "" {
		p.Subtitulos.ColorActivo = "&H0000E5FF&" // ámbar; en ASS el orden es BGR
	}
	if p.Subtitulos.EscalaPop == 0 {
		p.Subtitulos.EscalaPop = 112
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
	if p.Subtitulos.Activos {
		valida := false
		for _, a := range AnimacionesValidas {
			if p.Subtitulos.Animacion == a {
				valida = true
				break
			}
		}
		if !valida {
			return fmt.Errorf("perfil %s: subtitulos.animacion=%q no existe; use uno de: %s",
				p.ID, p.Subtitulos.Animacion, strings.Join(AnimacionesValidas, ", "))
		}
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

// Personaje superpone una figura fija que acompaña la narración. En temas de
// reflexión o consejo, una cara presente sostiene la atención mucho mejor que
// una sucesión de paisajes, aunque no diga nada por sí misma.
type Personaje struct {
	// Imagen es un PNG con transparencia, relativo a la carpeta del perfil.
	Imagen   string  `json:"imagen"`
	Posicion string  `json:"posicion"` // abajo-derecha | abajo-izquierda | abajo-centro
	AltoPct  float64 `json:"alto_pct"` // % del alto del video que ocupa
	Margen   int     `json:"margen"`   // px al borde
	Opacidad float64 `json:"opacidad"` // 1 = opaco
	// Animacion: ninguna | respirar | hablar.
	//   respirar – oscilación lenta, solo para que no parezca una calcomanía
	//   hablar   – se mueve durante los tramos con voz y se detiene en los
	//              silencios, usando los tiempos por palabra de whisper
	Animacion string `json:"animacion"`

	// Forma: circulo | recorte | croma | tarjeta.
	//   circulo – recorte circular. Funciona con cualquier imagen, sin preparar
	//   recorte – la imagen ya trae transparencia propia
	//   croma   – se le quita un color de fondo al vuelo
	Forma      string `json:"forma"`
	ColorCroma string `json:"color_croma"`
}
