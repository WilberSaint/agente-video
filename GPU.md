# Montarlo en un PC con GPU

Esta guía es para mover el agente a una máquina con tarjeta NVIDIA y generar
las imágenes en local.

**Los números están calculados para 6 GB de memoria dedicada**, que es lo que
muestra el administrador de tareas de esa máquina. Ojo con esto: la RTX 2060
**Super** lleva 8 GB, así que si ahí se ven 6 GB la tarjeta es una **2060 a
secas**. No es un problema —lo que sigue está pensado para 6 GB— pero conviene
saber con qué se cuenta antes de elegir modelo.

Con el escritorio abierto ya hay ~0,8 GB ocupados, así que lo aprovechable son
unos **5,2 GB**. Y no, no conviene apurarla al máximo: quedarse sin memoria a
media imagen aborta el video entero, y Windows tapa el problema volcando a la
"memoria compartida" —RAM del sistema— que funciona pero va varias veces más
lenta. Es mejor ir holgado y rápido que al límite y a trompicones.

## Por qué merece la pena

No es solo «se ven mejor». Son tres problemas concretos que desaparecen:

**La resolución.** Los generadores gratuitos ignoran el tamaño que se les pide.
Medido: Pollinations devuelve **576×1024** aunque se le pidan 1080×1920, y
Cloudflare devuelve **1024×1024 cuadrado**, que además pierde los lados al
recortarlo a vertical. Después el zoom Ken Burns amplía eso a 1620×2880 —casi
**3×**— y ahí se va el detalle. Ningún prompt arregla resolución que no existe.

**La velocidad.** Pollinations tarda ~45 s por imagen y admite **una petición
por IP a la vez**. Un video de 16 imágenes son 12 minutos, casi todos
esperando. En local no hay cola ajena ni límite de peticiones.

**La coherencia de personajes.** Depende de compartir semilla entre planos.
Cloudflare no la respeta. En local sí, siempre.

## Requisitos

| | |
|---|---|
| Driver NVIDIA | reciente; CUDA lo instala el propio WebUI |
| Python | 3.10 u 3.11 — **no 3.12+**, torch aún da guerra ahí |
| Git | para clonar |
| Go | 1.22+ para compilar el agente |
| Disco | ~15 GB entre modelo, entorno y videos |
| VRAM | 6 GB bastan con la configuración de abajo |

## 1. Clonar y compilar

```powershell
git clone https://github.com/WilberSaint/agente-video.git
cd agente-video
go build -o agente-video.exe .\cmd\agente-video
.\instalar.ps1        # ffmpeg, whisper.cpp, piper y voces
```

Copia tu `.env` a la raíz. Comprueba antes que sigue ignorado:

```powershell
git check-ignore -v .env      # tiene que responder con la regla
```

## 2. Stable Diffusion WebUI Forge

**Forge y no Automatic1111**: gestiona mejor la memoria y en 6 GB es la
diferencia entre generar y quedarse sin VRAM a media imagen. Con esta tarjeta
no es una preferencia, es lo que hace que SDXL entre.

```powershell
git clone https://github.com/lllyasviel/stable-diffusion-webui-forge.git
cd stable-diffusion-webui-forge
```

Edita `webui-user.bat` y pon:

```bat
set COMMANDLINE_ARGS=--api --xformers
```

`--api` es imprescindible: sin él el agente no tiene con qué hablar. Si la GPU
está en **otro equipo** de tu red, añade `--listen`.

Si con 6 GB da errores de memoria, añade `--always-offload-from-device`. Forge
va descargando de la tarjeta lo que no está usando; cuesta algo de velocidad y
quita casi todos los `CUDA out of memory`.

La primera ejecución baja torch y tarda un rato largo:

```powershell
.\webui-user.bat
```

## 3. El modelo

Un checkpoint **SDXL**. Se deja en `models\Stable-diffusion\`:

- **Juggernaut XL** — foto realista, lo que encaja con el estilo actual del
  perfil `historias` (cinematográfico, grano de 35 mm).
- **DreamShaper XL** — más ilustrado, si algún día quieres un canal con otro
  aire.

Y un ampliador para el segundo pase, en `models\ESRGAN\`: **4x-UltraSharp**.

## 4. Configurar el perfil

En `perfiles/historias/perfil.json`, el bloque `imagen`:

```json
"imagen": {
  "proveedor": "local",
  "servidor": "http://127.0.0.1:7860",
  "modelo": "juggernautXL_v9.safetensors",
  "ancho_base": 640,
  "ancho_tope": 896,
  "pasos": 28,
  "cfg": 5.5,
  "sampler": "DPM++ 2M Karras",
  "denoising": 0.35,
  "semilla": 3070,
  "estilo": "cinematic photography, moody volumetric lighting, 35mm film grain, desaturated teal and amber palette, shallow depth of field, no text",
  "negativo": "text, watermark, logo, deformed hands, extra limbs, low quality"
}
```

### Los dos números que importan en 6 GB

**`ancho_tope: 896`** — lo máximo que se le pide a la tarjeta, aunque el video
sea de 1080 de ancho. Pedirle 1080×1920 a SDXL con 6 GB se queda sin memoria;
896×1592 entra. Lo que falte hasta 1080 lo amplía ffmpeg. Puede sonar a
rendirse, pero **896 es casi el doble de los 576 que da Pollinations hoy**, y
esa es la comparación que cuenta.

**`ancho_base: 640`** — el agente pide **dos pasadas**: una a 640×1136, que va
sobrada, y otra que amplía a 896×1592 **añadiendo detalle**, no interpolando.
En una pasada sola a 896 la calidad es peor y el pico de memoria parecido, así
que partir sale ganando por los dos lados.

Si algún día pasas a una tarjeta de 12 GB o más: `ancho_tope: 0` y
`ancho_base: 768`, y ya genera a 1080×1920 nativo.

`denoising: 0.35` es bajo a propósito: el segundo pase debe **afinar** la
imagen que ya salió, no reinventarla. Por encima de 0,5 cambia la composición
y se pierde la coherencia que daba la semilla.

## 5. Comprobar antes de gastar tiempo

Con Forge corriendo, una sola imagen:

```powershell
.\agente-video.exe generar -perfil historias -tema "un sotano con una silla vacia"
```

Mira el registro: debe decir `local:juggernautXL...` y no `pollinations`.
Después comprueba la resolución real de lo que salió:

```powershell
.\bin\ffprobe.exe -v error -select_streams v `
  -show_entries stream=width,height -of csv=p=0 `
  trabajo\historias\<carpeta>\02-imagenes\escena-01-plano-1.png
```

**Con la configuración de arriba tiene que decir `896,1592`.** Si dice
`640,1136`, el segundo pase no se aplicó. Si dice 576 o 1024, sigue tirando de
Pollinations o Cloudflare y no has cambiado nada.

## 6. Ajustar con datos, no a ojo

No tengo forma de medir tu tarjeta desde aquí, así que estos son puntos de
partida, no promesas. Mide tú y ajusta:

- **Se queda sin memoria** → baja `ancho_tope` a 768, luego `ancho_base` a 576,
  y por último `pasos` a 20. Añade `--always-offload-from-device` a Forge.
- **Va sobrado de memoria** → sube `ancho_tope` a 960 y mide otra vez; cada
  escalón acerca la imagen a la resolución real del video.
- **Va muy lento** → baja `pasos`; entre 20 y 28 la diferencia se nota poco y
  el tiempo cae bastante.
- **Las caras salen deformes** → sube `pasos` y baja `cfg` a 4,5.
- **El segundo pase cambia demasiado la escena** → baja `denoising` a 0,25.

Para medir de verdad, cronometra un video entero y compáralo con los 12 min que
tarda hoy con Pollinations:

```powershell
Measure-Command { .\agente-video.exe generar -perfil historias -tema "..." }
```

## Lo que NO cambia al mudarse

- La voz sigue siendo ElevenLabs o Piper: nada de esto la afecta.
- Los guiones siguen viniendo de la API de Claude **o** de `guion.carpeta` con
  guiones escritos a mano.
- whisper.cpp y ffmpeg van por CPU. Irán más rápidos por ser mejor máquina,
  pero no usan la GPU.

## Un consejo de máquina, no de código

En el servidor actual, Windows Update arrancó a mitad de un video y dejó el
pipeline **297 segundos parado** — con el disco saturado, todo lo demás espera.
Antes de programar producción nocturna, dale a Windows Update una ventana de
mantenimiento que no choque con la hora del horario.

## Si algo falla

| Síntoma | Causa casi segura |
|---|---|
| `no se pudo hablar con la GPU en http://…` | Forge no está corriendo, o falta `--api` |
| `la GPU devolvió 404` | el nombre de `modelo` no coincide con el archivo |
| Imágenes en 640 de ancho | no se aplicó el segundo pase: `ancho_base` es mayor o igual que `ancho_tope` |
| `CUDA out of memory` en el segundo pase | `ancho_tope` demasiado alto para 6 GB; bájalo a 768 |
| `CUDA out of memory` | baja `ancho_base`; cierra lo que use la GPU |
| Sale lentísimo la primera vez | Forge compila kernels; a partir de la segunda va normal |
