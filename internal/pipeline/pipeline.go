// Package pipeline encadena las etapas con checkpoints en disco.
//
// Cada etapa escribe su resultado en la carpeta de trabajo. Si el archivo ya
// existe, la etapa se salta. Así un fallo en el render no obliga a pagar de
// nuevo el guion ni a regenerar las imágenes.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agente-video/internal/perfil"
	"agente-video/internal/proveedor"
)

type Proveedores struct {
	Guionista    proveedor.Guionista
	Imagenero    proveedor.Imagenero
	Locutor      proveedor.Locutor
	Subtitulador proveedor.Subtitulador
	Videasta     proveedor.Videasta
}

// Avance describe en qué punto va el trabajo. Existe aparte del registro de
// texto porque una interfaz necesita una fracción con la que dibujar una barra,
// y sacarla parseando líneas de log es frágil: cambiar un mensaje rompería la
// UI sin que nada avise.
type Avance struct {
	Etapa    int    `json:"etapa"` // 1..5
	Etiqueta string `json:"etiqueta"`
	Hecho    int    `json:"hecho"`
	Total    int    `json:"total"`
	Detalle  string `json:"detalle"`
}

// Fraccion es cuánto del trabajo total está hecho, de 0 a 1. Las etapas no
// duran lo mismo —las imágenes se llevan la mayor parte— así que se ponderan
// según lo medido, en vez de repartir 20% a cada una.
func (a Avance) Fraccion() float64 {
	pesos := [6]float64{0, 0.05, 0.70, 0.03, 0.05, 0.17} // índice = etapa
	var base float64
	for i := 1; i < a.Etapa && i < len(pesos); i++ {
		base += pesos[i]
	}
	dentro := 0.0
	if a.Total > 0 {
		dentro = float64(a.Hecho) / float64(a.Total)
	}
	if a.Etapa >= 1 && a.Etapa < len(pesos) {
		base += pesos[a.Etapa] * dentro
	}
	if base > 1 {
		return 1
	}
	return base
}

type Opciones struct {
	DirTrabajo string
	DirSalida  string
	Reintentos int
	Registro   func(formato string, args ...any)
	// Avance es opcional; solo lo usa el servidor.
	Avance func(Avance)
}

type Pipeline struct {
	prov Proveedores
	opt  Opciones
}

func Nuevo(prov Proveedores, opt Opciones) *Pipeline {
	if opt.Reintentos <= 0 {
		opt.Reintentos = 3
	}
	if opt.Registro == nil {
		opt.Registro = func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	}
	return &Pipeline{prov: prov, opt: opt}
}

type Resultado struct {
	Guion      *proveedor.GuionGenerado
	DirTrabajo string
	VideoFinal string
	Duracion   time.Duration
}

// Ejecutar corre las cinco etapas. idTrabajo permite reanudar un trabajo previo;
// si viene vacío se crea uno nuevo.
func (pl *Pipeline) Ejecutar(ctx context.Context, p *perfil.Perfil, tema, idTrabajo string) (*Resultado, error) {
	inicio := time.Now()

	if idTrabajo == "" {
		idTrabajo = fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), sanear(tema, 40))
	}
	dir := filepath.Join(pl.opt.DirTrabajo, p.ID, idTrabajo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	pl.opt.Registro("carpeta de trabajo: %s", dir)

	rutaGuion := filepath.Join(dir, "01-guion.json")
	dirImgs := filepath.Join(dir, "02-imagenes")
	rutaVoz := filepath.Join(dir, "03-voz.wav")
	rutaSRT := filepath.Join(dir, "04-palabras.srt")
	rutaVideo := filepath.Join(dir, "05-final.mp4")

	// --- Etapa 1: guion ---
	guion, err := pl.etapaGuion(ctx, p, tema, rutaGuion)
	if err != nil {
		return nil, fmt.Errorf("etapa 1 (guion): %w", err)
	}

	// --- Etapa 2: imágenes ---
	escenas, err := pl.etapaImagenes(ctx, p, guion, dirImgs)
	if err != nil {
		return nil, fmt.Errorf("etapa 2 (imágenes): %w", err)
	}

	// --- Etapa 3: voz ---
	if err := pl.etapaVoz(ctx, p, guion, rutaVoz); err != nil {
		return nil, fmt.Errorf("etapa 3 (voz): %w", err)
	}

	// --- Etapa 4: subtítulos ---
	if p.Subtitulos.Activos {
		if err := pl.etapaSubtitulos(ctx, p, rutaVoz, rutaSRT); err != nil {
			return nil, fmt.Errorf("etapa 4 (subtítulos): %w", err)
		}
	} else {
		pl.opt.Registro("[4/5] subtítulos desactivados en el perfil")
	}

	// --- Etapa 5: ensamblado ---
	if err := pl.etapaVideo(ctx, p, escenas, rutaVoz, rutaSRT, rutaVideo); err != nil {
		return nil, fmt.Errorf("etapa 5 (video): %w", err)
	}

	// Copia a salida/ con un nombre legible.
	final := filepath.Join(pl.opt.DirSalida, p.ID,
		fmt.Sprintf("%s-%s.mp4", time.Now().Format("20060102"), sanear(guion.Titulo, 60)))
	if err := copiar(rutaVideo, final); err != nil {
		pl.opt.Registro("aviso: no se pudo copiar a salida/: %v", err)
		final = rutaVideo
	}
	pl.escribirMetadatos(final, guion)

	return &Resultado{
		Guion:      guion,
		DirTrabajo: dir,
		VideoFinal: final,
		Duracion:   time.Since(inicio),
	}, nil
}

func (pl *Pipeline) etapaGuion(ctx context.Context, p *perfil.Perfil, tema, ruta string) (*proveedor.GuionGenerado, error) {
	if datos, err := os.ReadFile(ruta); err == nil {
		var g proveedor.GuionGenerado
		if json.Unmarshal(datos, &g) == nil && len(g.Escenas) > 0 {
			// Normalizar convierte los checkpoints del formato anterior, de una
			// imagen por escena, al de varios planos.
			g.Normalizar()
			pl.opt.Registro("[1/5] guion reutilizado del checkpoint (%d escenas, %d imágenes)",
				len(g.Escenas), g.TotalPlanos())
			return &g, nil
		}
	}
	pl.avanzar(1, "guion", 0, 1, "escribiendo el guion")
	pl.opt.Registro("[1/5] escribiendo guion con %s…", pl.prov.Guionista.Nombre())
	g, err := pl.prov.Guionista.Generar(ctx, p, tema)
	if err != nil {
		return nil, err
	}
	datos, _ := json.MarshalIndent(g, "", "  ")
	if err := os.WriteFile(ruta, datos, 0o644); err != nil {
		return nil, err
	}
	pl.opt.Registro("      «%s» — %d escenas, %d imágenes", g.Titulo, len(g.Escenas), g.TotalPlanos())
	return g, nil
}

func (pl *Pipeline) etapaImagenes(ctx context.Context, p *perfil.Perfil,
	g *proveedor.GuionGenerado, dir string) ([]proveedor.EscenaRender, error) {

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	escenas := make([]proveedor.EscenaRender, 0, len(g.Escenas))
	total := g.TotalPlanos()
	hechas := 0

	for _, esc := range g.Escenas {
		render := proveedor.EscenaRender{Narracion: esc.Narracion}

		for j, plano := range esc.Planos {
			hechas++
			// Sin extensión: el proveedor decide si sale PNG, JPEG o WebP.
			base := filepath.Join(dir, fmt.Sprintf("escena-%02d-plano-%d", esc.N, j+1))
			render.Encuadres = append(render.Encuadres, plano.Encuadre)

			if ya := buscarImagen(base); ya != "" {
				pl.opt.Registro("[2/5] imagen %d/%d ya existe, se salta", hechas, total)
				render.Imagenes = append(render.Imagenes, ya)
				continue
			}

			req := proveedor.PeticionImagen{
				Prompt:   componerPrompt(p, plano.Prompt),
				Negativo: p.Imagen.Negativo,
				Semilla:  plano.SemillaDe(p.Imagen.Semilla, esc.N, j),
				Ancho:    p.Formato.Ancho,
				Alto:     p.Formato.Alto,
				Destino:  base,
			}

			var escrita string
			var err error
			for intento := 1; intento <= pl.opt.Reintentos; intento++ {
				pl.avanzar(2, "imágenes", hechas-1, total, fmt.Sprintf("imagen %d de %d", hechas, total))
				pl.opt.Registro("[2/5] imagen %d/%d (escena %d, %s) con %s (intento %d)…",
					hechas, total, esc.N, encuadreONada(plano.Encuadre),
					pl.prov.Imagenero.Nombre(), intento)
				if escrita, err = pl.prov.Imagenero.Generar(ctx, req); err == nil {
					break
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				pl.opt.Registro("      falló: %v", err)
				time.Sleep(time.Duration(intento*3) * time.Second)
			}
			if err != nil {
				return nil, fmt.Errorf("escena %d plano %d tras %d intentos: %w",
					esc.N, j+1, pl.opt.Reintentos, err)
			}
			render.Imagenes = append(render.Imagenes, escrita)
		}
		escenas = append(escenas, render)
	}
	return escenas, nil
}

func encuadreONada(e string) string {
	if e == "" {
		return "sin encuadre"
	}
	return e
}

// buscarImagen devuelve el archivo de imagen ya generado para una escena,
// sea cual sea la extensión, ignorando descargas a medias.
func buscarImagen(base string) string {
	for _, ext := range []string{".png", ".jpg", ".webp"} {
		if info, err := os.Stat(base + ext); err == nil && info.Size() > 0 {
			return base + ext
		}
	}
	return ""
}

func (pl *Pipeline) etapaVoz(ctx context.Context, p *perfil.Perfil, g *proveedor.GuionGenerado, ruta string) error {
	if info, err := os.Stat(ruta); err == nil && info.Size() > 0 {
		pl.opt.Registro("[3/5] narración reutilizada del checkpoint")
		return nil
	}
	pl.avanzar(3, "voz", 0, 1, "sintetizando la narración")
	pl.opt.Registro("[3/5] sintetizando narración con %s…", pl.prov.Locutor.Nombre())
	return pl.prov.Locutor.Sintetizar(ctx, proveedor.PeticionVoz{
		Texto:     g.NarracionCompleta(),
		Modelo:    p.RutaRelativa(p.Voz.Modelo),
		Velocidad: p.Voz.Velocidad,
		Destino:   ruta,
	})
}

func (pl *Pipeline) etapaSubtitulos(ctx context.Context, p *perfil.Perfil, audio, ruta string) error {
	if info, err := os.Stat(ruta); err == nil && info.Size() > 0 {
		pl.opt.Registro("[4/5] subtítulos reutilizados del checkpoint")
		return nil
	}
	pl.avanzar(4, "subtítulos", 0, 1, "transcribiendo")
	pl.opt.Registro("[4/5] transcribiendo con %s…", pl.prov.Subtitulador.Nombre())
	return pl.prov.Subtitulador.Generar(ctx, proveedor.PeticionSubtitulos{
		Audio:      audio,
		Idioma:     strings.Split(p.Idioma, "-")[0],
		DestinoSRT: ruta,
	})
}

func (pl *Pipeline) etapaVideo(ctx context.Context, p *perfil.Perfil,
	escenas []proveedor.EscenaRender, audio, srt, destino string) error {

	if info, err := os.Stat(destino); err == nil && info.Size() > 0 {
		pl.opt.Registro("[5/5] video ya ensamblado, se salta")
		return nil
	}
	pl.avanzar(5, "montaje", 0, 1, "ensamblando el video")
	pl.opt.Registro("[5/5] ensamblando con %s (esta etapa es la lenta)…", pl.prov.Videasta.Nombre())
	return pl.prov.Videasta.Ensamblar(ctx, proveedor.PeticionVideo{
		Perfil:    p,
		Escenas:   escenas,
		Audio:     audio,
		SRT:       srt,
		Destino:   destino,
		MusicaSrc: p.RutaRelativa(p.Video.Musica),
	})
}

// componerPrompt une el prompt de la escena con el estilo y el personaje del
// perfil: eso es lo que hace que dos canales distintos con el mismo tema
// produzcan videos que no se parecen.
func componerPrompt(p *perfil.Perfil, prompt string) string {
	partes := []string{}
	if p.Imagen.Personaje != "" {
		partes = append(partes, p.Imagen.Personaje)
	}
	partes = append(partes, prompt)
	if p.Imagen.Estilo != "" {
		partes = append(partes, p.Imagen.Estilo)
	}
	return strings.Join(partes, ", ")
}

// escribirMetadatos deja título, descripción y hashtags junto al video, listos
// para copiar y pegar al publicar.
func (pl *Pipeline) escribirMetadatos(rutaVideo string, g *proveedor.GuionGenerado) {
	txt := fmt.Sprintf("%s\n\n%s\n\n%s\n", g.Titulo, g.Descripcion, hashtags(g.Hashtags))
	_ = os.WriteFile(strings.TrimSuffix(rutaVideo, ".mp4")+".txt", []byte(txt), 0o644)
}

func hashtags(hs []string) string {
	out := make([]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, "#"+strings.TrimPrefix(h, "#"))
	}
	return strings.Join(out, " ")
}

var noAlfanumerico = regexp.MustCompile("[^a-z0-9]+")

func sanear(s string, max int) string {
	s = strings.ToLower(strings.TrimSpace(s))
	reemplazos := map[rune]string{'á': "a", 'é': "e", 'í': "i", 'ó': "o", 'ú': "u", 'ñ': "n", 'ü': "u"}
	var b strings.Builder
	for _, r := range s {
		if v, ok := reemplazos[r]; ok {
			b.WriteString(v)
		} else {
			b.WriteRune(r)
		}
	}
	s = noAlfanumerico.ReplaceAllString(b.String(), "-")
	s = strings.Trim(s, "-")
	if len(s) > max {
		s = strings.Trim(s[:max], "-")
	}
	if s == "" {
		s = "video"
	}
	return s
}

func copiar(origen, destino string) error {
	if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
		return err
	}
	e, err := os.Open(origen)
	if err != nil {
		return err
	}
	defer e.Close()
	s, err := os.Create(destino)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = io.Copy(s, e)
	return err
}

// avanzar notifica el progreso si alguien está escuchando.
func (pl *Pipeline) avanzar(etapa int, etiqueta string, hecho, total int, detalle string) {
	if pl.opt.Avance == nil {
		return
	}
	pl.opt.Avance(Avance{Etapa: etapa, Etiqueta: etiqueta, Hecho: hecho, Total: total, Detalle: detalle})
}
