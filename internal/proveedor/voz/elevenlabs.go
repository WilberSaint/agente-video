package voz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agente-video/internal/herramientas"
	"agente-video/internal/proveedor"
)

// ElevenLabs sintetiza por API en vez de en local.
//
// Por qué esto y no generar los audios a mano y dejarlos en una carpeta: la
// narración se escribe nueva en cada video. Un banco de audios pregrabados solo
// serviría si el texto se repitiera, y entonces no habría agente. Con la API el
// texto que acaba de escribir el guionista se convierte en voz en el mismo
// paso, sin que nadie toque nada.
//
// La contrapartida es que se paga por carácter y hay cuota. Un video de 45
// segundos son unos 600 caracteres.
type ElevenLabs struct {
	// Aviso comunica ajustes que se han tenido que corregir. Si es nil, callan.
	Aviso func(string, ...any)

	apiKey string
	vozID  string
	modelo string
	http   *http.Client
}

func NuevoElevenLabs(apiKey, vozID, modelo string) *ElevenLabs {
	if modelo == "" {
		// Multilingual v2 es el que mejor pronuncia el español.
		modelo = "eleven_multilingual_v2"
	}
	return &ElevenLabs{
		apiKey: apiKey,
		vozID:  vozID,
		modelo: modelo,
		http:   &http.Client{Timeout: 4 * time.Minute},
	}
}

func (e *ElevenLabs) Nombre() string { return "elevenlabs:" + e.modelo }

type peticionEL struct {
	Texto   string    `json:"text"`
	Modelo  string    `json:"model_id"`
	Ajustes ajustesEL `json:"voice_settings"`
}

type ajustesEL struct {
	// Estabilidad baja da más expresividad y más variación entre frases; alta
	// da una lectura más plana pero predecible. 0.45 es un punto intermedio
	// que funciona para narración.
	Estabilidad float64 `json:"stability"`
	Similitud   float64 `json:"similarity_boost"`
	Estilo      float64 `json:"style"`
	// Velocidad la aplica ElevenLabs al generar, no estirando el audio
	// después: la voz suena más rápida, no acelerada.
	Velocidad float64 `json:"speed,omitempty"`
}

// La API rechaza con 422 cualquier velocidad fuera de este rango, y un perfil
// con un número copiado de Piper —donde la escala es otra— tumbaría el video
// entero por un detalle de configuración.
const (
	velocidadMin = 0.7
	velocidadMax = 1.2
)

func acotarVelocidad(v float64, avisar func(string, ...any)) float64 {
	if v <= 0 {
		return 1
	}
	if ajustada := math.Min(math.Max(v, velocidadMin), velocidadMax); ajustada != v {
		if avisar != nil {
			avisar("velocidad %.2f fuera del rango de ElevenLabs (%.1f–%.1f); se usa %.2f",
				v, velocidadMin, velocidadMax, ajustada)
		}
		return ajustada
	}
	return v
}

func (e *ElevenLabs) Sintetizar(ctx context.Context, req proveedor.PeticionVoz) error {
	if e.apiKey == "" {
		return fmt.Errorf("falta ELEVENLABS_API_KEY")
	}
	if e.vozID == "" {
		return fmt.Errorf("falta voz.modelo con el id de la voz de ElevenLabs " +
			"(lo copias de la web, en Voices)")
	}
	if err := os.MkdirAll(filepath.Dir(req.Destino), 0o755); err != nil {
		return err
	}

	cuerpo, err := json.Marshal(peticionEL{
		Texto:  req.Texto,
		Modelo: e.modelo,
		Ajustes: ajustesEL{
			// Estos tres están afinados para narración y no se exponen en el
			// perfil: expresividad y variacion son escalas de Piper y
			// significan otra cosa aquí. La velocidad sí, porque es lo que se
			// ajusta oyendo.
			Estabilidad: 0.45,
			Similitud:   0.80,
			Estilo:      0.15,
			Velocidad:   acotarVelocidad(req.Voz.Velocidad, e.Aviso),
		},
	})
	if err != nil {
		return err
	}

	url := "https://api.elevenlabs.io/v1/text-to-speech/" + e.vozID
	peticion, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(cuerpo))
	if err != nil {
		return err
	}
	peticion.Header.Set("xi-api-key", e.apiKey)
	peticion.Header.Set("Content-Type", "application/json")
	peticion.Header.Set("Accept", "audio/mpeg")

	respuesta, err := e.http.Do(peticion)
	if err != nil {
		return fmt.Errorf("elevenlabs: %w", err)
	}
	defer respuesta.Body.Close()

	if respuesta.StatusCode != http.StatusOK {
		detalle, _ := io.ReadAll(io.LimitReader(respuesta.Body, 400))
		err := fmt.Errorf("elevenlabs devolvió %d: %s", respuesta.StatusCode, detalle)
		if respuesta.StatusCode == http.StatusUnauthorized {
			return proveedor.Permanente(fmt.Errorf("%w\n\nRevisa ELEVENLABS_API_KEY", err))
		}
		if respuesta.StatusCode == 422 {
			return proveedor.Permanente(fmt.Errorf("%w\n\nSuele ser un id de voz "+
				"que no existe en tu cuenta", err))
		}
		if respuesta.StatusCode == http.StatusPaymentRequired {
			return proveedor.Permanente(fmt.Errorf("%w\n\nEl plan gratuito no deja usar "+
				"voces de la biblioteca por API; solo las premade de tu cuenta", err))
		}
		// Cualquier otro 4xx tampoco cambia reintentando: la petición es la que
		// no vale. Marcarlo permite al respaldo entrar en vez de tumbar el video.
		if respuesta.StatusCode >= 400 && respuesta.StatusCode < 500 &&
			respuesta.StatusCode != http.StatusTooManyRequests {
			return proveedor.Permanente(err)
		}
		return err
	}

	// Llega en MP3; el resto del pipeline trabaja con WAV, así que se convierte
	// aquí y no en el ensamblador, para que el formato sea uniforme desde el
	// principio y whisper reciba siempre lo mismo.
	mp3 := strings.TrimSuffix(req.Destino, ".wav") + ".el.mp3"
	f, err := os.Create(mp3)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, respuesta.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()

	destinoWav := req.Destino
	if req.Voz.Procesar {
		destinoWav = strings.TrimSuffix(req.Destino, ".wav") + ".crudo.wav"
	}
	if _, err := herramientas.Correr(ctx, "ffmpeg", "-y", "-loglevel", "error",
		"-i", mp3, "-ar", "22050", "-ac", "1", "-c:a", "pcm_s16le", destinoWav); err != nil {
		return fmt.Errorf("convirtiendo el audio de elevenlabs: %w", err)
	}
	_ = os.Remove(mp3)

	// El procesado se aplica igual que con Piper, aunque aquí haga menos falta:
	// normalizar a -16 LUFS mantiene el volumen coherente entre videos hechos
	// con proveedores distintos.
	if req.Voz.Procesar {
		return Mejorar(ctx, destinoWav, req.Destino, req.Voz)
	}
	return nil
}

var _ proveedor.Locutor = (*ElevenLabs)(nil)
