package imagen

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"agente-video/internal/proveedor"
)

// Pollinations: gratuito y sin llave. Es comunitario, así que puede ir lento
// o fallar; por eso el pipeline reintenta.
type Pollinations struct {
	modelo string
	http   *http.Client
}

func NuevoPollinations(modelo string) *Pollinations {
	if modelo == "" {
		modelo = "flux"
	}
	return &Pollinations{modelo: modelo, http: &http.Client{Timeout: clienteHTTPTimeout}}
}

func (p *Pollinations) Nombre() string { return "pollinations:" + p.modelo }

func (p *Pollinations) Generar(ctx context.Context, req proveedor.PeticionImagen) (string, error) {
	q := url.Values{}
	q.Set("width", strconv.Itoa(req.Ancho))
	q.Set("height", strconv.Itoa(req.Alto))
	q.Set("seed", strconv.FormatInt(req.Semilla, 10))
	q.Set("model", p.modelo)
	q.Set("nologo", "true")
	q.Set("enhance", "false")

	destinoURL := "https://image.pollinations.ai/prompt/" + url.PathEscape(req.Prompt) + "?" + q.Encode()

	peticion, err := http.NewRequestWithContext(ctx, http.MethodGet, destinoURL, nil)
	if err != nil {
		return "", err
	}
	peticion.Header.Set("User-Agent", "agente-video/1.0")

	respuesta, err := p.http.Do(peticion)
	if err != nil {
		return "", fmt.Errorf("pollinations: %w", err)
	}
	defer respuesta.Body.Close()

	if respuesta.StatusCode != http.StatusOK {
		cuerpo, _ := io.ReadAll(io.LimitReader(respuesta.Body, 400))
		return "", fmt.Errorf("pollinations devolvió %d: %s", respuesta.StatusCode, cuerpo)
	}
	datos, err := io.ReadAll(respuesta.Body)
	if err != nil {
		return "", err
	}
	return escribirArchivo(req.Destino, datos)
}

var _ proveedor.Imagenero = (*Pollinations)(nil)

// SoportaSemilla es true: Pollinations respeta el parámetro seed, y con él la
// coherencia entre planos del mismo sujeto.
func (p *Pollinations) SoportaSemilla() bool { return true }
