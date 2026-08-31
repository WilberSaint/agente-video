// agente-video genera videos verticales cortos a partir de un tema y un perfil.
//
//	agente-video perfiles
//	agente-video doctor
//	agente-video generar -perfil demo -tema "el faro que nadie visita"
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"agente-video/internal/herramientas"
	"agente-video/internal/perfil"
	"agente-video/internal/pipeline"
	"agente-video/internal/proveedor"
	"agente-video/internal/proveedor/guion"
	"agente-video/internal/proveedor/imagen"
	"agente-video/internal/proveedor/subtitulos"
	"agente-video/internal/proveedor/video"
	"agente-video/internal/proveedor/voz"
)

func main() {
	if len(os.Args) < 2 {
		uso()
		os.Exit(2)
	}

	// Ctrl+C cancela limpiamente: los checkpoints ya escritos se conservan.
	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	var err error
	switch os.Args[1] {
	case "generar":
		err = cmdGenerar(ctx, os.Args[2:])
	case "perfiles":
		err = cmdPerfiles(os.Args[2:])
	case "doctor":
		err = cmdDoctor(ctx)
	case "-h", "--help", "help", "ayuda":
		uso()
		return
	default:
		uso()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
}

func uso() {
	fmt.Print(`agente-video — genera videos verticales cortos desde un tema

  agente-video perfiles                       lista los perfiles disponibles
  agente-video doctor                         revisa binarios y credenciales
  agente-video generar -perfil X -tema "..."  genera un video

Opciones de "generar":
  -perfil    string  id del perfil (carpeta dentro de perfiles/)   [obligatorio]
  -tema      string  de qué trata el video                          [obligatorio]
  -trabajo   string  id de un trabajo previo, para reanudarlo
  -perfiles  string  carpeta de perfiles           (por defecto "perfiles")
  -trabajos  string  carpeta de checkpoints        (por defecto "trabajo")
  -salida    string  carpeta de videos terminados  (por defecto "salida")
  -bin       string  carpeta de binarios externos  (por defecto "bin")
  -reintentos int    reintentos por imagen         (por defecto 3)
  -animacion string  ninguna | pop | karaoke | palabra  (sobrescribe el perfil)

Variables de entorno:
  ANTHROPIC_API_KEY   llave para el guionista (obligatoria)
  CF_ACCOUNT_ID       cuenta de Cloudflare  (si imagen.proveedor = cloudflare)
  CF_API_TOKEN        token de Cloudflare   (si imagen.proveedor = cloudflare)
  WHISPER_MODELO      ruta al .bin de whisper.cpp (por defecto bin/modelos/ggml-base.bin)
`)
}

func cmdGenerar(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("generar", flag.ExitOnError)
	idPerfil := fs.String("perfil", "", "id del perfil")
	tema := fs.String("tema", "", "tema del video")
	idTrabajo := fs.String("trabajo", "", "id de trabajo a reanudar")
	dirPerfiles := fs.String("perfiles", "perfiles", "carpeta de perfiles")
	dirTrabajos := fs.String("trabajos", "trabajo", "carpeta de checkpoints")
	dirSalida := fs.String("salida", "salida", "carpeta de salida")
	dirBin := fs.String("bin", "bin", "carpeta de binarios")
	reintentos := fs.Int("reintentos", 3, "reintentos por imagen")
	animacion := fs.String("animacion", "", "sobrescribe subtitulos.animacion del perfil")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *idPerfil == "" || *tema == "" {
		fs.Usage()
		return fmt.Errorf("-perfil y -tema son obligatorios")
	}
	herramientas.DirBin = *dirBin

	p, err := perfil.Cargar(*dirPerfiles, *idPerfil)
	if err != nil {
		return err
	}
	// Override para comparar estilos de subtítulo sin tocar el perfil. Como el
	// video es la única etapa afectada, basta con borrar 05-final.mp4 para
	// re-renderizar en segundos reutilizando el resto de checkpoints.
	if *animacion != "" {
		p.Subtitulos.Animacion = *animacion
		if err := p.Validar(); err != nil {
			return err
		}
	}

	provs, err := construirProveedores(p)
	if err != nil {
		return err
	}

	fmt.Printf("perfil : %s (%s)\n", p.Nombre, p.ID)
	fmt.Printf("tema   : %s\n", *tema)
	fmt.Printf("formato: %dx%d @%dfps, %d escenas, ~%ds\n\n",
		p.Formato.Ancho, p.Formato.Alto, p.Formato.FPS, p.Guion.Escenas, p.Guion.DuracionSeg)

	registro := func(f string, a ...any) {
		fmt.Printf("%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(f, a...))
	}
	// Los avisos del render salen por el mismo canal que el resto del progreso.
	if kb, ok := provs.Videasta.(*video.KenBurns); ok {
		kb.Aviso = func(f string, a ...any) { registro("      aviso: "+f, a...) }
	}

	pl := pipeline.Nuevo(provs, pipeline.Opciones{
		DirTrabajo: *dirTrabajos,
		DirSalida:  *dirSalida,
		Reintentos: *reintentos,
		Registro:   registro,
	})

	res, err := pl.Ejecutar(ctx, p, *tema, *idTrabajo)
	if err != nil {
		return err
	}

	fmt.Printf("\nlisto en %s\n", res.Duracion.Round(time.Second))
	fmt.Printf("  video : %s\n", res.VideoFinal)
	fmt.Printf("  título: %s\n", res.Guion.Titulo)
	fmt.Printf("  textos: %s\n", strings.TrimSuffix(res.VideoFinal, ".mp4")+".txt")
	return nil
}

func cmdPerfiles(args []string) error {
	fs := flag.NewFlagSet("perfiles", flag.ExitOnError)
	dirPerfiles := fs.String("perfiles", "perfiles", "carpeta de perfiles")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ids, err := perfil.Listar(*dirPerfiles)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		fmt.Printf("no hay perfiles en %s/\n", *dirPerfiles)
		return nil
	}
	for _, id := range ids {
		p, err := perfil.Cargar(*dirPerfiles, id)
		if err != nil {
			fmt.Printf("  %-16s  (inválido: %v)\n", id, err)
			continue
		}
		fmt.Printf("  %-16s  %s — %dx%d, voz %s, imagen %s\n",
			p.ID, p.Nombre, p.Formato.Ancho, p.Formato.Alto, p.Voz.Proveedor, p.Imagen.Proveedor)
	}
	return nil
}

// cmdDoctor revisa todo lo que el pipeline necesita ANTES de gastar una llamada
// a la API. Es lo primero que hay que correr en una máquina nueva.
func cmdDoctor(ctx context.Context) error {
	fmt.Println("binarios externos")
	binarios := []struct{ nombre, para string }{
		{"ffmpeg", "ensamblado del video"},
		{"ffprobe", "medir duración del audio"},
		{"piper", "narración (voz)"},
		{"whisper-cli", "subtítulos"},
	}
	faltan := 0
	for _, b := range binarios {
		ruta, err := herramientas.Buscar(b.nombre)
		if err != nil {
			if b.nombre == "whisper-cli" {
				if r2, e2 := herramientas.Buscar("main"); e2 == nil {
					fmt.Printf("  [ok]    %-12s %s\n", b.nombre, r2)
					continue
				}
			}
			fmt.Printf("  [FALTA] %-12s (%s)\n", b.nombre, b.para)
			faltan++
			continue
		}
		fmt.Printf("  [ok]    %-12s %s\n", b.nombre, ruta)
	}

	fmt.Println("\ncredenciales")
	revisarVar("ANTHROPIC_API_KEY", true)
	revisarVar("CF_ACCOUNT_ID", false)
	revisarVar("CF_API_TOKEN", false)

	fmt.Println("\nmodelos")
	modeloWhisper := os.Getenv("WHISPER_MODELO")
	if modeloWhisper == "" {
		modeloWhisper = filepath.Join("bin", "modelos", "ggml-base.bin")
	}
	if _, err := os.Stat(modeloWhisper); err != nil {
		fmt.Printf("  [FALTA] modelo whisper  %s\n", modeloWhisper)
		faltan++
	} else {
		fmt.Printf("  [ok]    modelo whisper  %s\n", modeloWhisper)
	}

	// Las fuentes se revisan aquí porque su ausencia no rompe el render: hace
	// que los subtítulos desaparezcan en silencio, que es mucho peor.
	ids, _ := perfil.Listar("perfiles")
	fmt.Printf("\nperfiles: %d encontrados\n", len(ids))
	for _, id := range ids {
		p, err := perfil.Cargar("perfiles", id)
		if err != nil || !p.Subtitulos.Activos || p.Subtitulos.Fuente == "" {
			continue
		}
		if herramientas.FuenteInstalada(p.Subtitulos.Fuente) {
			fmt.Printf("  [ok]    %-12s fuente %q\n", id, p.Subtitulos.Fuente)
		} else {
			fmt.Printf("  [AVISO] %-12s la fuente %q no está instalada; "+
				"se usará la predeterminada\n", id, p.Subtitulos.Fuente)
		}
	}

	if faltan > 0 {
		return fmt.Errorf("faltan %d componentes; revisa el README para instalarlos", faltan)
	}
	fmt.Println("\ntodo listo.")
	return nil
}

func revisarVar(nombre string, obligatoria bool) {
	if os.Getenv(nombre) != "" {
		fmt.Printf("  [ok]    %s\n", nombre)
		return
	}
	if obligatoria {
		fmt.Printf("  [FALTA] %s\n", nombre)
	} else {
		fmt.Printf("  [--]    %s (opcional)\n", nombre)
	}
}

// construirProveedores traduce lo que dice el perfil a implementaciones
// concretas. Añadir un proveedor nuevo es añadir un case aquí.
func construirProveedores(p *perfil.Perfil) (pipeline.Proveedores, error) {
	var provs pipeline.Proveedores

	provs.Guionista = guion.NuevoClaude(os.Getenv("ANTHROPIC_API_KEY"), "")

	switch p.Imagen.Proveedor {
	case "cloudflare":
		provs.Imagenero = imagen.NuevoCloudflare(
			os.Getenv("CF_ACCOUNT_ID"), os.Getenv("CF_API_TOKEN"), p.Imagen.Modelo)
	case "pollinations", "":
		provs.Imagenero = imagen.NuevoPollinations(p.Imagen.Modelo)
	default:
		return provs, fmt.Errorf("proveedor de imagen desconocido: %q", p.Imagen.Proveedor)
	}

	switch p.Voz.Proveedor {
	case "piper", "":
		provs.Locutor = voz.NuevoPiper()
	default:
		return provs, fmt.Errorf("proveedor de voz desconocido: %q", p.Voz.Proveedor)
	}

	provs.Subtitulador = subtitulos.NuevoWhisper(os.Getenv("WHISPER_MODELO"))

	switch p.Video.Proveedor {
	case "kenburns", "":
		provs.Videasta = video.NuevoKenBurns()
	default:
		return provs, fmt.Errorf("proveedor de video desconocido: %q (aún no implementado)", p.Video.Proveedor)
	}

	var _ proveedor.Guionista = provs.Guionista
	return provs, nil
}
