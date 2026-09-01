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

	l := NuevoLocal(srv.URL, "juggernautXL.safetensors", "", 0, 0, 0, 0, 0)
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

	_, err := NuevoLocal(srv.URL, "", "", 0, 0, 0, 0, 0).Generar(context.Background(),
		proveedor.PeticionImagen{Destino: filepath.Join(t.TempDir(), "x")})
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if !proveedor.EsPermanente(err) {
		t.Errorf("un 404 debería ser permanente, no reintentable: %v", err)
	}
}

func TestLocalRespetaLaSemilla(t *testing.T) {
	if !NuevoLocal("", "", "", 0, 0, 0, 0, 0).SoportaSemilla() {
		t.Error("SoportaSemilla debe ser true: es la razón de usar GPU propia")
	}
}

// Una tarjeta de 8 GB no genera 1080x1920 de una pasada con SDXL. Con
// ancho_base se genera pequeño y se amplía, y lo que no puede cambiar es el
// tamaño FINAL: si no coincide con el formato del perfil, el zoom vuelve a
// ampliar y se pierde lo ganado.
func TestLocalPideDosPasadasCuandoLaTarjetaNoDaParaUna(t *testing.T) {
	var recibido peticionTxt2Img
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&recibido)
		_ = json.NewEncoder(w).Encode(respuestaTxt2Img{Imagenes: []string{pngMinimo}})
	}))
	defer srv.Close()

	l := NuevoLocal(srv.URL, "", "", 0, 768, 0, 0, 0)
	_, err := l.Generar(context.Background(), proveedor.PeticionImagen{
		Ancho: 1080, Alto: 1920, Destino: filepath.Join(t.TempDir(), "p"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !recibido.SegundoPase {
		t.Fatal("no pidió el segundo pase; en 8 GB eso es un error de memoria")
	}
	if recibido.Ancho != 768 {
		t.Errorf("primera pasada de %d px de ancho, se esperaban 768", recibido.Ancho)
	}
	// La primera pasada tiene que conservar la proporción del formato: si no,
	// el segundo pase deforma la imagen al estirarla.
	if recibido.Alto != 1360 {
		t.Errorf("alto de la primera pasada = %d; 768*1920/1080 redondeado a 8 son 1360", recibido.Alto)
	}
	if recibido.HRAncho != 1080 || recibido.HRAlto != 1920 {
		t.Errorf("tamaño final %dx%d, se esperaba 1080x1920", recibido.HRAncho, recibido.HRAlto)
	}
	if recibido.Denoising <= 0 || recibido.Denoising > 0.5 {
		t.Errorf("denoising %v: por encima de 0.5 el segundo pase recompone la imagen",
			recibido.Denoising)
	}
}

// Si la tarjeta da para el tamaño completo, partir en dos solo añade tiempo.
func TestLocalNoPideSegundoPaseSiNoHaceFalta(t *testing.T) {
	var recibido peticionTxt2Img
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&recibido)
		_ = json.NewEncoder(w).Encode(respuestaTxt2Img{Imagenes: []string{pngMinimo}})
	}))
	defer srv.Close()

	for _, base := range []int{0, 1080, 2000} {
		recibido = peticionTxt2Img{}
		l := NuevoLocal(srv.URL, "", "", 0, base, 0, 0, 0)
		if _, err := l.Generar(context.Background(), proveedor.PeticionImagen{
			Ancho: 1080, Alto: 1920, Destino: filepath.Join(t.TempDir(), "p"),
		}); err != nil {
			t.Fatal(err)
		}
		if recibido.SegundoPase {
			t.Errorf("ancho_base=%d: pidió segundo pase sin necesidad", base)
		}
		if recibido.Ancho != 1080 || recibido.Alto != 1920 {
			t.Errorf("ancho_base=%d: generó %dx%d", base, recibido.Ancho, recibido.Alto)
		}
	}
}

// Con 6 GB no se puede pedir 1080x1920: se queda sin memoria. El tope recorta
// lo que se le pide a la tarjeta manteniendo la proporción, y ffmpeg amplía el
// resto. Sigue siendo casi el doble que los 576 de los servicios gratuitos.
func TestLocalRespetaElTopeDeLaTarjeta(t *testing.T) {
	var recibido peticionTxt2Img
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&recibido)
		_ = json.NewEncoder(w).Encode(respuestaTxt2Img{Imagenes: []string{pngMinimo}})
	}))
	defer srv.Close()

	l := NuevoLocal(srv.URL, "", "", 0, 640, 896, 0, 0)
	if _, err := l.Generar(context.Background(), proveedor.PeticionImagen{
		Ancho: 1080, Alto: 1920, Destino: filepath.Join(t.TempDir(), "p"),
	}); err != nil {
		t.Fatal(err)
	}
	if recibido.HRAncho != 896 {
		t.Errorf("tamaño final %d, se esperaba el tope de 896", recibido.HRAncho)
	}
	// 896*1920/1080 = 1592,9 -> 1592, múltiplo de 8. Si la proporción se
	// pierde, la imagen sale deformada al montarla en vertical.
	if recibido.HRAlto != 1592 {
		t.Errorf("alto final %d, se esperaba 1592 para no deformar el 9:16", recibido.HRAlto)
	}
	if recibido.Ancho != 640 {
		t.Errorf("primera pasada de %d, se esperaba 640", recibido.Ancho)
	}
}
