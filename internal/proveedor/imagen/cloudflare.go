package imagen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"agente-video/internal/proveedor"
)

// Cloudflare Workers AI (FLUX.1-schnell). Tiene tier gratuito diario y API
// oficial con llave, así que es más estable que Pollinations.
// Requiere CF_ACCOUNT_ID y CF_API_TOKEN.
type Cloudflare struct {
	cuenta string
	token  string
	modelo string
	http   *http.Client
}

func NuevoCloudflare(cuenta, token, modelo string) *Cloudflare {
	if modelo == "" {
		modelo = "@cf/black-forest-labs/flux-1-schnell"
	}
	return &Cloudflare{cuenta: cuenta, token: token, modelo: modelo,
		http: &http.Client{Timeout: clienteHTTPTimeout}}
}

func (c *Cloudflare) Nombre() string { return "cloudflare:" + c.modelo }

type respuestaCF struct {
	Result struct {
		Image string `json:"image"` // base64
	} `json:"result"`
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Cloudflare) Generar(ctx context.Context, req proveedor.PeticionImagen) (string, error) {
	if c.cuenta == "" || c.token == "" {
		return "", fmt.Errorf("cloudflare: faltan CF_ACCOUNT_ID y/o CF_API_TOKEN")
	}

	// Sin "seed": flux-1-schnell en Workers AI rechaza cualquier propiedad que
	// no esté en su esquema y responde 400. Comprobado contra la API: acepta
	// "prompt" y "steps", nada más. La consecuencia es que en este proveedor no
	// se puede fijar la semilla, y con ella se pierde la coherencia de
	// personajes entre planos.
	cuerpo, err := json.Marshal(map[string]any{
		"prompt": req.Prompt,
		"steps":  4, // schnell está afinado para 4 pasos
	})
	if err != nil {
		return "", err
	}

	destinoURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/ai/run/%s",
		c.cuenta, c.modelo)

	peticion, err := http.NewRequestWithContext(ctx, http.MethodPost, destinoURL, bytes.NewReader(cuerpo))
	if err != nil {
		return "", err
	}
	peticion.Header.Set("Authorization", "Bearer "+c.token)
	peticion.Header.Set("Content-Type", "application/json")

	respuesta, err := c.http.Do(peticion)
	if err != nil {
		return "", fmt.Errorf("cloudflare: %w", err)
	}
	defer respuesta.Body.Close()

	datos, err := io.ReadAll(respuesta.Body)
	if err != nil {
		return "", err
	}
	if respuesta.StatusCode != http.StatusOK {
		err := fmt.Errorf("cloudflare devolvió %d: %s", respuesta.StatusCode, recorta(datos, 400))
		// Un 4xx que no sea 429 significa que la petición está mal: reintentarla
		// da exactamente el mismo error y entierra la causa.
		if respuesta.StatusCode >= 400 && respuesta.StatusCode < 500 && respuesta.StatusCode != http.StatusTooManyRequests {
			return "", proveedor.Permanente(err)
		}
		return "", err
	}

	var r respuestaCF
	if err := json.Unmarshal(datos, &r); err != nil {
		return "", fmt.Errorf("cloudflare: respuesta ilegible: %w", err)
	}
	if !r.Success {
		msg := "sin detalle"
		if len(r.Errors) > 0 {
			msg = r.Errors[0].Message
		}
		return "", fmt.Errorf("cloudflare rechazó la petición: %s", msg)
	}

	imagen, err := base64.StdEncoding.DecodeString(r.Result.Image)
	if err != nil {
		return "", fmt.Errorf("cloudflare: base64 inválido: %w", err)
	}
	return escribirArchivo(req.Destino, imagen)
}

func recorta(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

var _ proveedor.Imagenero = (*Cloudflare)(nil)

// SoportaSemilla es false: flux-1-schnell en Workers AI no admite el parámetro.
// Lo que se pierde es la coherencia de personajes entre planos, que se conseguía
// compartiendo semilla.
func (c *Cloudflare) SoportaSemilla() bool { return false }
