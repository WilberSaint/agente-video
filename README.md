| `piper` | Aceptable, algo sintética | Tu CPU | Gratis, ilimitado |
| **`kokoro`** | **Bastante mejor** | Tu CPU | **Gratis, ilimitado** |
| `elevenlabs` | La mejor | Su nube | Por carácter |# agente-video

Genera videos verticales cortos (TikTok / Reels / Shorts) a partir de **un tema
y un perfil**: guion, imágenes, narración, subtítulos y montaje, sin intervención
manual.

Corre entero en CPU. El único costo por video son los centavos del guion.

```
tema + perfil
     │
     ├─ 1. guion ........ Claude escribe narración + un prompt visual por escena
     ├─ 2. imágenes ..... una por escena (Pollinations o Cloudflare)
     ├─ 3. voz .......... Piper, local
     ├─ 4. subtítulos ... whisper.cpp, local
     └─ 5. montaje ...... ffmpeg: Ken Burns + transiciones + subs + música
                                    │
                             salida/<perfil>/*.mp4 + *.txt
```

---

## La restricción que define el diseño

El sistema **solo produce imágenes fijas**. No genera video en movimiento ni
animación: nada se mueve dentro de una imagen. Todo lo demás se deriva de ahí.

El dinamismo no viene del movimiento, viene de la edición:

| Recurso | Cómo se aplica |
|---|---|
| **Cambio de plano** | 1-3 imágenes por idea, con encuadres distintos entre sí |
| **Ritmo del relato** | Cada imagen dura lo que dura su parte de la narración, no un intervalo fijo |
| **Movimiento simulado** | Ken Burns, con intensidad ajustada al encuadre |
| **Subtítulos** | Grandes, sincronizados palabra a palabra, con animación de entrada |
| **Narración** | Cada frase aporta información, tensión, emoción o curiosidad |

Dos consecuencias que el guionista tiene prohibido violar:

1. **Nunca una narración que dependa de ver movimiento** ("mira cómo corre").
   Eso no se va a ver nunca.
2. **Cada imagen debe sostenerse sola** y representar con claridad lo que se
   dice en ese instante. Un instante congelado y elocuente, no un fotograma
   cualquiera de una secuencia.

Por eso los temas que mejor funcionan son los que se representan bien en imagen
fija: historias, datos, misterios, psicología, reflexiones y conceptos.

### Las imágenes cambian cuando cambia la idea

Repartir la duración en partes iguales produce un video donde las imágenes
cambian a destiempo del relato: una idea corta se queda fija demasiado y una
larga se corta a media frase. Se nota, y se nota como error de edición.

Como whisper ya entrega el tiempo de cada palabra, cada escena se alinea con el
tramo de audio donde realmente se narra, y ese tramo se reparte entre sus
planos. `min_seg_por_imagen` y `max_seg_por_imagen` acotan el resultado: una
escena con mucha narración y un solo plano dejaría la imagen congelada.

No se comparan textos para alinear —whisper transcribe lo que oye, y "cuarenta"
puede volver como "40"— sino la proporción en palabras de cada escena.

### Audio: narración, música y efectos

En un video de imágenes fijas el audio no es decoración. Cuando la imagen cambia
y no se oye nada, el corte se lee como un salto; con un barrido corto encima se
lee como una decisión de edición. Es el mismo corte: cambia que el oído lo
acompaña.

```
assets/musica/     tus pistas (no entra al repo)
assets/efectos/    generados por .\generar-efectos.ps1
```

Los efectos **se sintetizan con ffmpeg** en vez de descargarse: no dependen de
la licencia de nadie y se reproducen en cualquier máquina.

```powershell
.\generar-efectos.ps1     # whoosh, impacto, riser, clic
```

En el perfil:

| Campo | Efecto |
|---|---|
| `musica` | ruta a la pista; se repite sola si es más corta que la narración |
| `volumen_musica` | 0.12 va bien: debe oírse sin competir con la voz |
| `efecto_transicion` | el `.wav` que suena en cada corte |
| `volumen_efectos` | 0.30 es audible sin ser molesto |
| `efectos_en` | `escena` (al cambiar de idea), `plano` (cada imagen) o `ninguno` |

`escena` es el valor por defecto y casi siempre el correcto: dentro de una
escena los planos siguen hablando de lo mismo, y marcarlos todos cansa.

La mezcla suma con `normalize=0` para que la narración no pierda volumen cada
vez que se añade una pista, y cierra con un limitador para no saturar.

> **Licencias:** revisa lo que pones en `assets/musica/`. Una pista comercial
> dispara Content ID en YouTube y TikTok aunque el video sea tuyo. Pixabay,
> Free Music Archive y la biblioteca de audio de YouTube son seguras.

### Coherencia entre planos

Cada prompt se genera **por separado y sin memoria de los demás**. Lo que no se
repita explícitamente no se mantiene. Por eso el guionista tiene instrucción de
repetir la descripción física de personajes, lugares, época e iluminación con
las mismas palabras exactas en cada prompt donde aparezcan, en vez de escribir
"the same man": el generador no sabe a quién se refiere.

Pero repetir la descripción **no basta**. Medido sobre un caso real, tres planos
con el mismo texto de personaje y semillas distintas dieron un abrigo beige, uno
verde oscuro y una cara deformada.

Lo que sí funciona es **compartir la semilla**. Cada plano lleva un campo
`sujeto` con un identificador corto y estable ("mujer", "farero"), y todos los
planos que lo comparten se generan con la misma semilla. Repitiendo el mismo
experimento con semilla compartida, los tres planos salieron con el mismo corte
de pelo, el mismo abrigo beige y sin deformaciones.

Dos apoyos más:

- **Encuadre**: las caras se deforman cuando la persona ocupa poco de la imagen,
  así que el guionista reserva el plano `general` para lugares y ambientes, y
  usa `medio`/`cercano`/`detalle` cuando se ve una cara.
- `imagen.personaje` en el perfil hace lo mismo a nivel de canal, para un
  personaje fijo que aparece en todos los videos.

Aun así no es infalible: los generadores libres no tienen bloqueo de identidad.
Si el contenido no necesita personajes recurrentes, narrar sobre escenarios y
atmósfera da un resultado más sólido.

## Por qué está diseñado así

**El agente no sabe de temáticas.** Toda la personalidad vive en el perfil: el
estilo visual, la voz, el tono narrativo, el formato, el personaje recurrente.
El mismo binario sirve a diez canales distintos sin tocar una línea de código —
solo se agrega una carpeta en `perfiles/`.

**Cada etapa deja checkpoint en disco.** Si el render falla en el minuto 6, al
reintentar se reutilizan el guion, las imágenes y la voz ya generados. Nunca se
paga dos veces por lo mismo.

**Los proveedores están detrás de interfaces.** Cambiar Ken Burns por Sora, o
Piper por ElevenLabs, es escribir un tipo nuevo que cumpla la interfaz y agregar
un `case` en `construirProveedores`. El pipeline no se entera.

---

## Instalación

Requiere **Go 1.22+**. Todo lo demás lo baja el instalador dentro de `bin/`,
sin tocar el sistema ni el PATH.

```powershell
.\instalar.ps1              # ffmpeg, whisper.cpp + modelo, piper + voz
go build -o agente-video.exe .\cmd\agente-video
.\agente-video.exe doctor   # verifica binarios, modelos y credenciales
```

Opciones del instalador:

```powershell
.\instalar.ps1 -ModeloWhisper small     # más preciso, ~3x más lento
.\instalar.ps1 -Voz es_ES-sharvard-medium
.\instalar.ps1 -Forzar                  # re-descarga todo
```

### Credenciales

El guionista construye el cliente sin pasarle credenciales explícitas, así que
sirve **cualquiera** de los métodos que resuelve el SDK. Hace falta uno:

| Método | Cómo | Cuándo conviene |
|---|---|---|
| Llave de API | `ANTHROPIC_API_KEY` | Servidor propio, scripts locales. Lo normal aquí |
| Token | `ANTHROPIC_AUTH_TOKEN` | Tokens de corta duración emitidos por ti |
| Perfil OAuth | `ant auth login` | Uso interactivo; no deja una llave estática en el entorno |
| Federación de identidades | 4 variables (abajo) | **CI/CD y nubes.** Sin secretos que guardar |

```powershell
[Environment]::SetEnvironmentVariable("ANTHROPIC_API_KEY","sk-ant-...","User")
```

Opcionales: `CF_ACCOUNT_ID` / `CF_API_TOKEN` (solo si usas Cloudflare para las
imágenes) y `WHISPER_MODELO` (ruta a otro `.bin`).

`agente-video doctor` dice **cuál se va a aplicar**, no solo si existe.

#### Federación de identidades

No hay llave que guardar: el proveedor de identidad emite un token de corta
duración y el SDK lo canjea. Requiere las cuatro:

```
ANTHROPIC_FEDERATION_RULE_ID
ANTHROPIC_ORGANIZATION_ID
ANTHROPIC_SERVICE_ACCOUNT_ID
ANTHROPIC_IDENTITY_TOKEN_FILE   (o ANTHROPIC_IDENTITY_TOKEN)
```

Necesita un emisor de identidad real —GCP, AWS, Azure o GitHub Actions—, así
que **en un servidor propio sin proveedor de identidad no aplica**: ahí sigue
haciendo falta una llave. Donde sí paga es al mover la generación a CI.

> **La trampa que cuesta una tarde:** `ANTHROPIC_API_KEY` **definida pero vacía**
> gana la precedencia sobre la federación y sobre el perfil de `ant`, y deja
> todo muerto sin dar ningún error. No la dejes en blanco: elimínala.
> `doctor` detecta este caso y lo señala.

---

## El panel

```powershell
.gente-video.exe servir
# panel en http://127.0.0.1:8787
```

**El panel es un programa que tiene que estar corriendo.** Mientras esa ventana
esté abierta, el panel responde y el horario vigila; al cerrarla, todo se
detiene. Lo que hay en disco no se pierde —videos, banco de temas, reglas y
cola— y un video a medio generar se reencola y retoma desde sus checkpoints.

### Que arranque solo con Windows

Un horario a las tres de la mañana no sirve de nada si el programa se apagó al
cerrar sesión. Como administrador:

```powershell
.\instalar-servicio.ps1          # registra una tarea que arranca con el sistema
.\instalar-servicio.ps1 -Quitar  # la elimina
```

Se usa una tarea programada y no un servicio de Windows porque un ejecutable
normal no habla el protocolo de servicios: registrarlo como tal exigiría un
envoltorio tipo NSSM. La tarea consigue lo mismo —arranca sin sesión iniciada,
sobrevive a reinicios y se reintenta sola si falla— sin añadir dependencias.

Eliges perfil, escribes **un tema por línea** y pulsa Generar. Los trabajos se
encolan y se procesan **de uno en uno**, con la barra de progreso actualizándose
en vivo. Al terminar, el video se reproduce en la misma página.

Escribir cinco temas y encolarlos de golpe es la diferencia entre lanzar un
video y dejar la máquina trabajando toda la noche.

| Opción | |
|---|---|
| `-puerto 8787` | puerto donde escuchar |
| `-direccion 0.0.0.0` | abrirlo a la red local (por defecto solo esta máquina) |

**No hay autenticación.** Por eso escucha solo en `127.0.0.1` salvo que lo
cambies a mano: cualquiera con acceso a ese puerto puede gastar tu saldo de API.
Si lo abres a la red, ponlo detrás de algo que pida credenciales.

### Cómo está montado

- **Vue 3 + Vite** en `interfaz/`, compilado a `dist/` y **empotrado en el
  binario** con `go:embed`. Desplegar sigue siendo copiar un `.exe`: no hay
  carpeta que sincronizar ni forma de que la interfaz y la API se desfasen.
- **Un solo obrero** procesa la cola. Es deliberado: el montaje satura los
  cuatro núcleos, así que dos videos a la vez no van al doble de velocidad, van
  los dos a la mitad.
- **SSE** para el progreso, no WebSocket: el flujo va en un solo sentido y
  `EventSource` reconecta solo, sin código de reconexión en el cliente.
- **La cola se persiste.** Si el servidor se cae a media generación, al volver
  el trabajo se reencola en lugar de quedar mintiendo en "corriendo".
- El progreso viaja **estructurado**, no parseando líneas de log: cambiar un
  mensaje no debe romper la barra sin que nada avise.

### Desarrollar la interfaz

```powershell
cd interfaz
npm install
npm run dev     # http://localhost:5173, con recarga en caliente
```

Vite redirige `/api` y `/media` al servidor Go en el 8787, así que se desarrolla
contra datos reales. Para incorporar los cambios al binario:

```powershell
npm run build
cd ..
go build -o agente-video.exe .\cmd\agente-video
```

---

## Uso por terminal

El panel no sustituye a la línea de comandos; ambos usan el mismo pipeline.

```powershell
.\agente-video.exe perfiles
.\agente-video.exe generar -perfil demo -tema "el faro que nadie visita"
```

Al terminar quedan dos archivos en `salida/demo/`: el `.mp4` y un `.txt` con
título, descripción y hashtags listos para pegar al publicar.

**Reanudar un trabajo interrumpido** — el id sale del nombre de la carpeta en
`trabajo/<perfil>/`:

```powershell
.\agente-video.exe generar -perfil demo -tema "el faro que nadie visita" `
                           -trabajo 20260831-143022-el-faro-que-nadie-visita
```

Ctrl+C corta limpio: lo ya generado se conserva.

---

## El perfil

Un perfil es una carpeta en `perfiles/<id>/` con un `perfil.json`. Los campos
que cambian el resultado de forma notoria:

| Campo | Efecto |
|---|---|
| `guion.tono` | la voz narrativa. Es lo que más distingue un canal de otro |
| `guion.escenas` | número de imágenes. Más escenas = más ritmo y más render |
| `imagen.estilo` | se añade a **todos** los prompts. Define la identidad visual |
| `imagen.semilla` | fija = escenas más coherentes entre sí; cámbiala para otra veta |
| `imagen.personaje` | descripción física repetida en cada prompt, para personajes recurrentes |
| `video.zoom` | intensidad del Ken Burns. 1.10 es sutil, 1.30 es agresivo |
| `subtitulos.margen_v` | altura de los subtítulos; súbelo para que no los tape la UI de la app |
| `subtitulos.animacion` | cómo aparecen los subtítulos. Ver abajo |

Para crear un perfil nuevo, copia `perfiles/demo/` y edita el `id` y el `nombre`.

---

## Subtítulos animados

whisper entrega tiempos **por palabra**, así que los subtítulos se animan sin
librerías extra: el ASS se genera con etiquetas de override y libass las
renderiza. Cuatro modos, en `subtitulos.animacion`:

| Modo | Qué hace | Cuándo usarlo |
|---|---|---|
| `pop` | La línea entra pequeña, se pasa de tamaño y se asienta | **Predeterminado.** Se ve vivo sin distraer |
| `karaoke` | La línea se queda y se resalta en color la palabra que se dice | Narración densa, donde ayuda leer la frase completa |
| `palabra` | Una sola palabra a la vez, grande y centrada | Máxima retención. El más agresivo |
| `ninguna` | Estáticos | Cuando el contenido manda y el texto solo acompaña |

Se comparan sin editar el perfil, y como solo cambia la etapa 5 el re-render
tarda segundos:

```powershell
Remove-Item trabajo\demo\<id-trabajo>\05-final.mp4
.\agente-video.exe generar -perfil demo -tema "..." -trabajo <id-trabajo> -animacion karaoke
```

Ajustes finos: `escala_pop` (cuánto se pasa el rebote; 112 es discreto, 130
exagerado), `palabras_por_linea` (4 va bien en vertical) y `color_activo` para
karaoke. **Ojo con el color**: en ASS el orden es `&HBBGGRR&`, azul-verde-rojo
al revés de lo habitual — `&H0000E5FF&` es ámbar, no azul.

En modo `palabra` conviene subir `tam_px` bastante (110-140): hay una sola
palabra en pantalla y debe llenar el ancho.

### Sobre la consistencia de personajes

Los generadores de imagen gratuitos no garantizan que el mismo personaje se vea
igual en ocho escenas. Dos mitigaciones, en orden de efectividad:

1. **Evitarlo**: dejar `imagen.personaje` vacío y narrar sobre escenarios y
   atmósfera. Es lo que hace la mayoría de los canales de historias, y el
   resultado es más sólido.
2. **Anclarlo**: escribir en `imagen.personaje` una descripción física detallada
   e invariable. Ayuda, pero no es perfecto.

---

## Estructura

```
cmd/agente-video/       CLI: generar, perfiles, doctor
internal/perfil/        carga y validación de perfiles
internal/proveedor/     las interfaces — el contrato del sistema
    guion/claude.go     guionista (Anthropic SDK, claude-opus-5)
    imagen/             pollinations.go, cloudflare.go
    voz/piper.go        síntesis local
    subtitulos/whisper.go
    video/kenburns.go   el grafo de filtros de ffmpeg
internal/pipeline/      las 5 etapas y sus checkpoints
internal/herramientas/  ejecución de binarios externos, ffprobe
perfiles/               un directorio por canal o persona
trabajo/                checkpoints (ignorado por git)
salida/                 videos terminados (ignorado por git)
bin/                    binarios y modelos (ignorado por git)
```

---

## Pruebas

```powershell
go test ./...                                          # todo
go test ./internal/proveedor/imagen -run EnVivo -v     # pega contra el servicio real
go test -short ./...                                   # salta las que necesitan red
```

Para juzgar un cambio en el prompt del guionista hace falta ver varios guiones
seguidos, y esperar tres renders son cuarenta minutos y las imágenes de los tres.
`probar-guion` pide los guiones y enseña solo el texto de publicación:

```powershell
go run .\cmd\probar-guion historias "un tema" "otro tema" "un tercero"
```

Es la forma de detectar que todos los videos cierran con la misma frase, que no
se ve mirando uno solo.

---

## Rendimiento

Medido en un Xeon E3-1220 v3 (4 núcleos, sin GPU), video vertical de ~45 s:

| Etapa | Tiempo |
|---|---|
| Guion | ~15 s |
| Imágenes | ~2-3 s cada una |
| Narración | ~30 s |
| Subtítulos | ~15 s |
| **Montaje** | **3-8 min** ← el cuello de botella |

El montaje domina porque `zoompan` es intensivo en CPU. Para acelerarlo:
bajar `factorEscala` en `kenburns.go`, subir el `Preset` de x264, o reducir el
número de escenas. Es una etapa desatendida: la forma correcta de usarlo es
encolar varios videos y dejarlos correr.

---

## Añadir un proveedor nuevo

Ejemplo, un generador de video real (Sora, Kling, Runway) cuando llegue el momento:

1. Crear `internal/proveedor/video/kling.go` con un tipo que cumpla
   `proveedor.Videasta`.
2. Agregar el `case "kling"` en `construirProveedores` (`cmd/agente-video/main.go`).
3. Poner `"proveedor": "kling"` en el `video` del perfil.

Nada más cambia. Los perfiles que sigan en `kenburns` no se enteran.

---

## La voz

Un TTS crudo suena plano por tres razones concretas, y el procesado ataca cada
una: le sobra grave y le falta presencia, la dinámica es irregular (unas sílabas
salen bastante más fuertes que otras), y las eses salen afiladas porque no hay
un micrófono real que las suavice.

```json
"voz": {
  "modelo": "../../bin/voces/es_MX-claude-high.onnx",
  "velocidad": 1.0,
  "expresividad": 0.95,
  "variacion": 0.72,
  "procesar": true,
  "presencia": 3.5
}
```

| Campo | Qué hace |
|---|---|
| `expresividad` | Varía la duración de cada fonema. **Es lo que más combate lo robótico**: con todos los fonemas midiendo casi lo mismo, la voz cae en cadencia de metrónomo. 0.8 es el neutro de Piper, 0.95 va bien, por encima de 1.0 se desestabiliza |
| `variacion` | Afecta al timbre. Pasado 0.8 empieza a sonar ebrio |
| `procesar` | Ecualización, de-esser, compresión y normalización a −16 LUFS |
| `presencia` | dB de realce en 3.2 kHz. Sube la inteligibilidad, sobre todo con música encima |

Con `procesar` activo, el audio sin tratar queda al lado como `03-voz.crudo.wav`
para poder comparar.

### Comparar voces

`muestras-voz/comparar.html` tiene el mismo texto en las cuatro voces
instaladas, cruda y procesada. Ábrelo en el navegador y escucha con auriculares:
en altavoces de portátil la diferencia casi no se aprecia.

Para instalar otra voz, de [piper-voices](https://huggingface.co/rhasspy/piper-voices):

```powershell
.\instalar.ps1 -Voz es_ES-davefx-medium
```

**El siguiente escalón es Kokoro**: bastante mejor que Piper, también gratis y
en CPU, pero necesita Python instalado. Piper se eligió porque permitía llegar a
un video terminado sin añadir un runtime entero.

---

## Personaje narrador

Superpone una figura fija que acompaña la narración. En temas de reflexión o
consejo, una cara presente sostiene la atención mucho mejor que una sucesión de
paisajes.

```json
"personaje": {
  "imagen": "personaje/narrador.jpg",
  "forma": "circulo",
  "posicion": "abajo-derecha",
  "alto_pct": 19,
  "margen": 44,
  "opacidad": 1,
  "animacion": "hablar"
}
```

**No hay sincronía labial** — eso exige un modelo y una GPU. Pero sí tenemos los
tiempos de cada palabra, así que el personaje **se mueve mientras se habla y se
detiene en los silencios**. El ojo lee ese acoplamiento como "está narrando"
aunque la boca no cambie: es el truco de los muñecos de guiñol. Sin esa
sincronía —moviéndose siempre igual— queda una calcomanía animada encima.

| Campo | |
|---|---|
| `forma` | `circulo` (por defecto), `recorte`, `croma`, `tarjeta` |
| `animacion` | `hablar`, `respirar` (oscilación lenta), `ninguna` |
| `alto_pct` | % del alto del video. **18-22 va bien**; más grande compite con la escena |
| `posicion` | `abajo-derecha`, `abajo-izquierda`, `abajo-centro` |

### Por qué `circulo` es el modo por defecto

Un PNG bien recortado es lo ideal, pero hay que prepararlo. Probado con
generadores libres: ignoran la instrucción de fondo croma, y **cuando la
respetan visten al personaje del mismo color**, con lo que el recorte se come la
ropa. El círculo funciona con cualquier imagen sin preparación —una foto tuya,
un avatar, un fotograma— y en vertical se lee como decisión de diseño.

Si tienes un PNG con transparencia de verdad, usa `"forma": "recorte"`.

**Los subtítulos se suben solos** para no quedar bajo el personaje. Sin eso el
círculo los corta, y es un fallo que no da ningún error: solo se ve mirando el
video terminado.

---

## Probar sin gastar créditos

**El guionista es la única etapa que cuesta dinero.** Imágenes, voz, subtítulos
y montaje son gratis. Al ajustar el personaje, los subtítulos, la música o el
ritmo hacen falta muchas pasadas, y pagar un guion nuevo cada vez para tirarlo a
los diez segundos no tiene sentido.

```powershell
.\agente-video.exe generar -perfil demo -tema "prueba" -simular
```

Usa un guion fijo pensado para que las pruebas sean representativas: escenas de
duración desigual, un sujeto recurrente, encuadres variados y una pausa larga
—eso último ejercita el corte de línea de los subtítulos y los tramos de
silencio del personaje, que es donde suelen aparecer los fallos. Se ajusta al
número de escenas del perfil.

Para reproducir un caso concreto que falló, se le puede pasar un guion guardado:

```powershell
.\agente-video.exe generar -perfil demo -tema "x" -guion trabajo\demo\<id>\01-guion.json
```

Y para iterar solo sobre el montaje, borrar `05-final.mp4` de un trabajo ya
hecho y relanzarlo con `-trabajo <id>` re-renderiza en ~30 segundos
reutilizando guion, imágenes y voz.

## Voz por API: ElevenLabs

```json
"voz": {
  "proveedor": "elevenlabs",
  "modelo": "<Voice ID copiado de la web, en Voices>",
  "procesar": true
}
```

Con `ELEVENLABS_API_KEY` en el entorno.

**Por qué la API y no audios pregrabados en una carpeta:** la narración se
escribe nueva en cada video. Un banco de audios solo serviría si el texto se
repitiera, y entonces no habría agente.

Un video de 45 s son unos 600 caracteres. El plan gratuito da 10.000 al mes
(~16 videos) **sin derechos de uso comercial**; el de 5 USD da 30.000 (~50).

| Proveedor | Calidad | Corre en | Costo |
|---|---|---|---|
| `piper` | Aceptable, algo sintética | Tu CPU | Gratis, ilimitado |
| `elevenlabs` | La mejor | Su nube | Por carácter |

El salto grande de calidad está en el modelo, no en el hardware: **Kokoro** corre
también en CPU y suena bastante mejor que Piper. Pide Python instalado, que es la
razón por la que no se incluyó de entrada.

### Kokoro — la voz local de mejor calidad

Gratis, ilimitada y en CPU, con una entonación bastante más natural que Piper.
Necesita Python, que es la razón por la que no viene de entrada.

```powershell
.\instalar-python.ps1     # Python autocontenido en bin\python, sin tocar el sistema
.\instalar-kokoro.ps1     # modelo (310 MB) y voces (27 MB)
```

```json
"voz": { "proveedor": "kokoro", "modelo": "em_alex", "procesar": true }
```

Voces: `em_alex` y `em_santa` (masculinas), `ef_dora` (femenina).

Sintetiza a unas dos veces el tiempo real frente a las diez de Piper —medido,
8.5 s de audio en 4.8 s— pero en un video de varios minutos veinte segundos más
no cambian nada.

Compara todas en `muestras-voz/comparar.html`.

---

## Producción automática

Es lo que separa una herramienta de un agente: sin esto, alguien tiene que estar
delante escribiendo el tema. La pestaña **Producción automática** del panel tiene
las dos piezas.

### Banco de temas

Pegas ideas —una por línea— y quedan guardadas por perfil. Los repetidos se
detectan comparando el texto normalizado, porque pegar dos veces la misma lista
es de lo más normal y duplicarlos produciría dos videos idénticos.

Cada tema pasa por **pendiente → usado**, y se puede descartar o reutilizar. El
panel muestra cuántos días de producción quedan: *"12 pendientes · da para 4
días a 3/día"* dice mucho más que un número suelto.

### Horario

Una regla es *"a las 03:00, 3 videos del perfil historias"*, opcionalmente
limitada a ciertos días. A esa hora el agente toma los temas del banco y los
encola.

| Detalle | Por qué |
|---|---|
| Se comprueba cada minuto | Sobrevive a reinicios, hibernación y cambios de hora; un cálculo hecho una vez se queda desfasado |
| Ventana de gracia de 1 hora | Si el servidor estaba caído a la hora exacta, aún se recupera. Pasado eso se deja pasar el día: producir a destiempo es peor que no producir |
| Una vez al día, por fecha | Comparar fechas y no intervalos evita disparar dos veces si el reloj salta |
| Los temas se marcan al tomarlos | Consultar primero y marcar después haría que dos disparos simultáneos tomaran los mismos |
| Si el trabajo no se encola, el tema vuelve a pendiente | La idea sigue siendo buena; lo que falló fue la máquina |

### Proponer temas

Con `proponer_si_faltan` activo, cuando el banco no da para la tanda el agente
pide ideas nuevas antes de rendirse. Se le pasan los temas ya publicados para
que no repita.

Sin esto, un banco vacío a las tres de la mañana es una noche perdida.
