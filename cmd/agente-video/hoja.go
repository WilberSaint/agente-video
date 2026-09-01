package main

import (
	"fmt"
	"path/filepath"

	"agente-video/internal/hoja"
)

// cmdExpresiones parte una hoja de expresiones en imágenes sueltas.
//
// Existe porque una hoja generada de una sola vez da expresiones del MISMO
// personaje —es literalmente la misma imagen—, mientras que generarlas por
// separado siempre produce deriva. Recortarlas a mano es tedioso y se hace mal.
func cmdExpresiones(args []string) error {
	fs := flagSet("expresiones")
	entrada := fs.String("hoja", "", "imagen con la rejilla de expresiones [obligatorio]")
	destino := fs.String("destino", "", "carpeta donde dejarlas [obligatorio]")
	umbral := fs.Int("umbral", 232, "luminancia por debajo de la cual un píxel es dibujo (0-255)")
	minLado := fs.Int("min-lado", 60, "descarta recortes menores a este tamaño, en píxeles")
	margen := fs.Int("margen", 4, "píxeles a recortar de cada panel")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *entrada == "" || *destino == "" {
		fs.Usage()
		return fmt.Errorf("-hoja y -destino son obligatorios")
	}

	opt := hoja.PorDefecto()
	opt.UmbralClaro = uint8(*umbral)
	opt.MinLado = *minLado
	opt.Margen = *margen

	escritos, err := hoja.Partir(*entrada, *destino, opt)
	if err != nil {
		return err
	}

	fmt.Printf("%d expresión(es) en %s\n", len(escritos), *destino)
	for _, r := range escritos {
		fmt.Printf("  %s\n", filepath.Base(r))
	}
	fmt.Printf("\nApunta el perfil a la carpeta para que roten por escena:\n")
	fmt.Printf("  \"personaje\": { \"imagen\": %q, ... }\n", filepath.ToSlash(*destino))
	return nil
}
