package imagen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agente-video/internal/proveedor"
)

// Local genera con una GPU propia, hablando con la API de Stable Diffusion
// WebUI (Automatic1111 o Forge) en /sdapi/v1/txt2img.
//
// Se eligió esa API y no la de ComfyUI porque es una sola petición JSON con
// los parámetros planos; ComfyUI exige mandar el grafo entero del flujo, que
// habría metido la configuración del modelo dentro del código Go.
//
// Lo que aporta frente a los generadores gratuitos no es solo calidad: es que
// la imagen sale con la resolución que se pide —los servicios libres devuelven
// 576x1024 o 1024x1024 cuadrado, y el zoom luego los amplía casi tres veces— y
// que la semilla se respeta, que es lo único que hace que un personaje se
// parezca a sí mismo entre planos.
type Local struct {
	base    string
	modelo  string
	pasos   int
	cfg     float64
	sampler string
	http    *http.Client
}

// NuevoLocal apunta a la GPU. base es la dirección del servidor, por ejemplo
// http://127.0.0.1:7860 en la misma máquina o http://192.168.1.50:7860 si la
// GPU está en otro equipo de la red.
func NuevoLocal(base, modelo, sampler string, pasos int, cfg float64) *Local {
	if base == "" {
		base = "http://127.0.0.1:7860"
	}
	if pasos <= 0 {
		pasos = 28
	}
	if cfg <= 0 {
		cfg = 5.5
	}
	if sampler == "" {
		sampler = "DPM++ 2M Karras"
	}
	return &Local{
		base:    strings.TrimRight(base, "/"),
		modelo:  modelo,
		pasos:   pasos,
		cfg:     cfg,
		sampler: sampler,
		// Generoso: en una tarjeta modesta una imagen vertical grande puede
		// pasar del minuto, y cortarla a medias desperdicia el trabajo hecho.
		http: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (l *Local) Nombre() string {
	if l.modelo == "" {
		return "local(sd-webui)"
	}
	return "local:" + l.modelo
}

// SoportaSemilla es true, y es la razón principal para preferirlo: con la
// misma semilla el mismo sujeto sale igual en todos los planos.
func (l *Local) SoportaSemilla() bool { return true }

type peticionTxt2Img struct {
	Prompt         string         `json:"prompt"`
	PromptNegativo string         `json:"negative_prompt,omitempty"`
	Semilla        int64          `json:"seed"`
	Pasos          int            `json:"steps"`
	CFG            float64        `json:"cfg_scale"`
	Ancho          int            `json:"width"`
	Alto           int            `json:"height"`
	Sampler        string         `json:"sampler_name,omitempty"`
	Ajustes        map[string]any `json:"override_settings,omitempty"`
	// Sin esto el modelo elegido se quedaría cargado y afectaría a quien use
	// la interfaz web después.
	RestaurarAjustes bool `json:"override_settings_restore_afterwards"`
}

type respuestaTxt2Img struct {
	Imagenes []string `json:"images"`
	Detalle  any      `json:"detail"`
}

func (l *Local) Generar(ctx context.Context, req proveedor.PeticionImagen) (string, error) {
	cuerpo := peticionTxt2Img{
		Prompt:           req.Prompt,
		PromptNegativo:   req.Negativo,
		Semilla:          req.Semilla,
		Pasos:            l.pasos,
		CFG:              l.cfg,
		Ancho:            req.Ancho,
		Alto:             req.Alto,
		Sampler:          l.sampler,
		RestaurarAjustes: true,
	}
	if l.modelo != "" {
		cuerpo.Ajustes = map[string]any{"sd_model_checkpoint": l.modelo}
	}

	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return "", err
	}

	peticion, err := http.NewRequestWithContext(ctx, http.MethodPost,
		l.base+"/sdapi/v1/txt2img", bytes.NewReader(datos))
	if err != nil {
		return "", err
	}
	peticion.Header.Set("Content-Type", "application/json")

	respuesta, err := l.http.Do(peticion)
	if err != nil {
		return "", fmt.Errorf("no se pudo hablar con la GPU en %s: %w\n\n"+
			"Arranca Stable Diffusion WebUI con --api (y --listen si está en otro equipo)", l.base, err)
	}
	defer respuesta.Body.Close()

	if respuesta.StatusCode != http.StatusOK {
		detalle, _ := io.ReadAll(io.LimitReader(respuesta.Body, 400))
		err := fmt.Errorf("la GPU devolvió %d: %s", respuesta.StatusCode, detalle)
		// Un modelo que no existe o un sampler mal escrito no se arreglan
		// reintentando; abortar cuanto antes evita repetir el mismo fallo.
		if respuesta.StatusCode >= 400 && respuesta.StatusCode < 500 {
			return "", proveedor.Permanente(err)
		}
		return "", err
	}

	var r respuestaTxt2Img
	if err := json.NewDecoder(respuesta.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("respuesta ilegible de la GPU: %w", err)
	}
	if len(r.Imagenes) == 0 {
		return "", fmt.Errorf("la GPU no devolvió ninguna imagen (detalle: %v)", r.Detalle)
	}

	// Llega en base64; a veces con el prefijo "data:image/png;base64,".
	b64 := r.Imagenes[0]
	if i := strings.Index(b64, ","); i >= 0 && strings.HasPrefix(b64, "data:") {
		b64 = b64[i+1:]
	}
	crudo, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("la imagen no venía en base64 válido: %w", err)
	}
	return escribirArchivo(req.Destino, crudo)
}

var _ proveedor.Imagenero = (*Local)(nil)
