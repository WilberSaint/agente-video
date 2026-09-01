// probar-guion pide varios guiones seguidos y enseña solo el texto de
// publicación. Sirve para ver si los cierres se repiten entre videos sin
// pagar las imágenes ni esperar el render, que es donde se va el tiempo.
package main

import (
	"context"
	"fmt"
	"os"

	"agente-video/internal/credenciales"
	"agente-video/internal/perfil"
	"agente-video/internal/proveedor/guion"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("uso: probar-guion <perfil> <tema> [tema...]")
		os.Exit(2)
	}
	_, _ = credenciales.CargarEnv(".env")

	p, err := perfil.Cargar("perfiles", os.Args[1])
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	g := guion.NuevoClaude(os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ANTHROPIC_WORKSPACE_ID"), "")

	for _, tema := range os.Args[2:] {
		gen, err := g.Generar(context.Background(), p, tema)
		if err != nil {
			fmt.Printf("· %s\n  error: %v\n\n", tema, err)
			continue
		}
		fmt.Printf("%s\n%s\n\n", gen.TituloPublicable(), gen.DescripcionPublicable())
	}
}
