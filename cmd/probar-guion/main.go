// probar-guion pide varios guiones seguidos y enseña solo el texto de
// publicación. Sirve para ver si los cierres se repiten entre videos sin
// pagar las imágenes ni esperar el render, que es donde se va el tiempo.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agente-video/internal/credenciales"
	"agente-video/internal/perfil"
	"agente-video/internal/proveedor/guion"
)

func main() {
	guardar := flag.String("guardar", "", "carpeta donde dejar cada guion y su narración")
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Println("uso: probar-guion [-guardar carpeta] <perfil> <tema> [tema...]")
		os.Exit(2)
	}
	_, _ = credenciales.CargarEnv(".env")

	p, err := perfil.Cargar("perfiles", flag.Arg(0))
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	g := guion.NuevoClaude(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ANTHROPIC_WORKSPACE_ID"), "")

	for _, tema := range flag.Args()[1:] {
		gen, err := g.Generar(context.Background(), p, tema)
		if err != nil {
			fmt.Printf("· %s\n  error: %v\n\n", tema, err)
			continue
		}
		fmt.Printf("%s\n%s\n\n", gen.TituloPublicable(), gen.DescripcionPublicable())

		if *guardar == "" {
			continue
		}
		// La narración se deja aparte del JSON porque es lo que se pega en un
		// sintetizador de fuera; el JSON es lo que luego arma el video sin
		// volver a pagar el guion.
		if err := os.MkdirAll(*guardar, 0o755); err != nil {
			fmt.Println("error:", err)
			continue
		}
		base := filepath.Join(*guardar, sanear(gen.Titulo))
		datos, _ := json.MarshalIndent(gen, "", "  ")
		_ = os.WriteFile(base+".json", datos, 0o644)
		_ = os.WriteFile(base+".txt", []byte(gen.NarracionCompleta()+"\n"), 0o644)
		fmt.Printf("  guardado en %s.{json,txt} — %d caracteres de narración\n\n",
			base, len([]rune(gen.NarracionCompleta())))
	}
}

// sanear deja un nombre de archivo utilizable a partir del título.
func sanear(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
