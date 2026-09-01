package voz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
			Estabilidad: 0.45,
			Similitud:   0.80,
			Estilo:      0.15,
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
