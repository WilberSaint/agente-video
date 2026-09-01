package guion

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agente-video/internal/perfil"
	"agente-video/internal/proveedor"
)

type guionistaContado struct {
	veces int
	err   error
}

func (g *guionistaContado) Nombre() string { return "contado" }
func (g *guionistaContado) Generar(context.Context, *perfil.Perfil, string) (*proveedor.GuionGenerado, error) {
	g.veces++
	if g.err != nil {
		return nil, g.err
	}
	return &proveedor.GuionGenerado{
		Titulo:  "escrito por el respaldo",
		Escenas: []proveedor.Escena{{Narracion: "algo", Planos: []proveedor.Plano{{Prompt: "x"}}}},
	}, nil
}

func escribirGuion(t *testing.T, dir, ranura, titulo string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	json := `{"titulo":"` + titulo + `","descripcion":"d","hashtags":["a"],
	  "escenas":[{"narracion":"n","planos":[{"prompt":"p","encuadre":"general"}]}]}`
	if err := os.WriteFile(filepath.Join(dir, ranura+".json"), []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCarpetaUsaElGuionEscritoYNoLlamaAlRespaldo(t *testing.T) {
	dir := t.TempDir()
	// El tema lleva acentos y mayúsculas; la ranura tiene que encontrarlo igual.
	escribirGuion(t, dir, "el-retrato-que-envejecia", "hecho a mano")

	resp := &guionistaContado{}
	c := NuevoCarpeta(dir, resp)
	g, err := c.Generar(context.Background(), &perfil.Perfil{}, "El Retrato que envejecía")
	if err != nil {
		t.Fatal(err)
	}
	if g.Titulo != "hecho a mano" {
		t.Errorf("usó %q en vez del guion de la carpeta", g.Titulo)
	}
	if resp.veces != 0 {
		t.Errorf("llamó al respaldo %d veces: eso es dinero gastado de más", resp.veces)
	}
}

func TestCarpetaCaeAlRespaldoSiNoHayGuion(t *testing.T) {
	resp := &guionistaContado{}
	c := NuevoCarpeta(t.TempDir(), resp)
	g, err := c.Generar(context.Background(), &perfil.Perfil{}, "un tema sin guion escrito")
	if err != nil {
		t.Fatal(err)
	}
	if g.Titulo != "escrito por el respaldo" || resp.veces != 1 {
		t.Errorf("no entró el respaldo: titulo=%q veces=%d", g.Titulo, resp.veces)
	}
}

// Sin respaldo, quedarse callado sería peor: el video fallaría con un error
// oscuro en vez de decir dónde falta el archivo.
func TestCarpetaSinRespaldoDiceDondeFaltaElArchivo(t *testing.T) {
	dir := t.TempDir()
	_, err := NuevoCarpeta(dir, nil).Generar(context.Background(), &perfil.Perfil{}, "tema ausente")
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	if !proveedor.EsPermanente(err) {
		t.Error("un guion que falta no aparece reintentando; debe ser permanente")
	}
	if !strings.Contains(err.Error(), "tema-ausente.json") {
		t.Errorf("el error no dice qué archivo falta: %v", err)
	}
}

func TestCarpetaAbortaConUnJSONRoto(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "tema-roto.json"), []byte("{no es json"), 0o644)
	resp := &guionistaContado{}
	_, err := NuevoCarpeta(dir, resp).Generar(context.Background(), &perfil.Perfil{}, "tema roto")
	if err == nil {
		t.Fatal("se esperaba un error")
	}
	// Ojo: NO debe caer al respaldo. Un archivo roto es un descuido que hay que
	// ver y arreglar, no algo que se tape gastando dinero en silencio.
	if resp.veces != 0 {
		t.Error("tapó un guion roto llamando al respaldo")
	}
	if !proveedor.EsPermanente(err) {
		t.Error("un JSON roto no se arregla reintentando; debe ser permanente")
	}
}

func TestRanura(t *testing.T) {
	casos := map[string]string{
		"El Retrato que envejecía": "el-retrato-que-envejecia",
		"  dos   espacios  ":       "dos-espacios",
		"¿Qué pasó en 1978?":       "que-paso-en-1978",
		"acentos: ñandú, corazón":  "acentos-nandu-corazon",
	}
	for dado, quiere := range casos {
		if got := Ranura(dado); got != quiere {
			t.Errorf("Ranura(%q) = %q, se esperaba %q", dado, got, quiere)
		}
	}
}
