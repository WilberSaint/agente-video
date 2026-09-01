package imagen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"agente-video/internal/proveedor"
)

// pngMinimo es un PNG de 1x1 válido, suficiente para comprobar que lo que
// llega en base64 acaba escrito en disco.
const pngMinimo = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestLocalMandaLoQueSeLePideYGuardaLaImagen(t *testing.T) {
	var recibido peticionTxt2Img
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sdapi/v1/txt2img" {
			t.Errorf("llamó a %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&recibido)
		_ = json.NewEncoder(w).Encode(respuestaTxt2Img{Imagenes: []string{pngMinimo}})
	}))
	defer srv.Close()

	l := NuevoLocal(srv.URL, "juggernautXL.safetensors", "", 0, 0)
	destino := filepath.Join(t.TempDir(), "plano")
	ruta, err := l.Generar(context.Background(), proveedor.PeticionImagen{
		Prompt: "a dark basement", Negativo: "text", Semilla: 4242,
		Ancho: 832, Alto: 1472, Destino: destino,
	})
	if err != nil {
		t.Fatal(err)
	}

	// La resolución pedida es el motivo de existir de este proveedor: si no
	// llega tal cual, el zoom acaba ampliando una imagen pequeña.
	if recibido.Ancho != 832 || recibido.Alto != 1472 {
		t.Errorf("pidió %dx%d, se esperaba 832x1472", recibido.Ancho, recibido.Alto)
	}
	if recibido.Semilla != 4242 {
		t.Errorf("semilla = %d; sin ella no hay coherencia de personaje", recibido.Semilla)
	}
	if recibido.PromptNegativo != "text" {
		t.Errorf("no se mandó el negativo: %q", recibido.PromptNegativo)
	}
	if !recibido.RestaurarAjustes {
		t.Error("debe restaurar los ajustes: si no, deja el modelo cambiado en la web")
	}

	datos, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("no se escribió la imagen: %v", err)
	}
	esperado, _ := base64.StdEncoding.DecodeString(pngMinimo)
	if len(datos) != len(esperado) {
		t.Errorf("la imagen guardada mide %d bytes, se esperaban %d", len(datos), len(esperado))
	}
}

// Un modelo mal escrito devuelve 404 y no se arregla reintentando: hay que
// abortar en el primer intento y no repetir el error tres veces.
func TestLocalAbortaConUnErrorDelCliente(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"model not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := NuevoLocal(srv.URL, "", "", 0, 0).Generar(context.Background(),
		proveedor.PeticionImagen{Destino: filepath.Join(t.TempDir(), "x")})
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if !proveedor.EsPermanente(err) {
		t.Errorf("un 404 debería ser permanente, no reintentable: %v", err)
	}
}

func TestLocalRespetaLaSemilla(t *testing.T) {
	if !NuevoLocal("", "", "", 0, 0).SoportaSemilla() {
		t.Error("SoportaSemilla debe ser true: es la razón de usar GPU propia")
	}
}
