# instalar-python.ps1 — deja un Python autocontenido en bin\python\
#
# Se usa la distribución "embeddable" en lugar del instalador oficial: es un ZIP
# que no toca el sistema, no pide permisos de administrador y no depende de que
# el instalador funcione en Windows Server. Queda dentro de bin\, igual que
# ffmpeg, whisper y piper, así que borrar esa carpeta lo revierte todo.
#
# La contrapartida es que la embeddable viene SIN pip a propósito, y con las
# rutas de import recortadas. Este script arregla las dos cosas.
#
#   .\instalar-python.ps1
#   .\instalar-python.ps1 -Forzar

[CmdletBinding()]
param(
    [switch]$Forzar,
    [string]$Version = "3.12.7"
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Raiz    = $PSScriptRoot
$Destino = Join-Path $Raiz "bin\python"
$Exe     = Join-Path $Destino "python.exe"
$Temp    = Join-Path $Raiz "bin\.descargas"

# Se comprueba pip y no solo python.exe: la distribución embeddable trae el
# ejecutable pero NO pip, así que dar por completa la instalación al ver el
# .exe deja el entorno inservible sin que nada lo advierta.
#
# La comprobación se hace mirando el disco y no ejecutando "python -m pip":
# en PowerShell 5.1, redirigir el stderr de un ejecutable nativo convierte su
# salida en un error de terminación y, con ErrorActionPreference en Stop, aborta
# el script aunque el comando fuera solo una consulta.
$pipInstalado = Test-Path (Join-Path $Destino "Lib\site-packages\pip")
if ((Test-Path $Exe) -and $pipInstalado -and -not $Forzar) {
    Write-Host "Python ya está completo en $Destino"
    & $Exe --version
    & $Exe -m pip --version
    exit 0
}
if ((Test-Path $Exe) -and -not $pipInstalado) {
    Write-Host "Python está en $Destino pero le falta pip; se completa."
}

New-Item -ItemType Directory -Path $Destino, $Temp -Force | Out-Null

# Si el intérprete ya está, no hace falta volver a descargarlo.
$soloPip = (Test-Path $Exe) -and -not $Forzar

function Bajar([string]$url, [string]$salida, [string]$etiqueta) {
    Write-Host "  bajando $etiqueta..." -NoNewline
    try {
        Invoke-WebRequest -Uri $url -OutFile $salida -UseBasicParsing -MaximumRedirection 10
    } catch {
        Write-Host " FALLO"
        throw "no se pudo descargar $etiqueta desde $url : $(($_.Exception.Message -split "`n")[0])"
    }
    $mb = [math]::Round((Get-Item $salida).Length / 1MB, 1)
    Write-Host " ok ($mb MB)"
}

Write-Host ""
if (-not $soloPip) {
Write-Host "[1/4] Python $Version (embeddable)"
$zip = Join-Path $Temp "python-embed.zip"
Bajar "https://www.python.org/ftp/python/$Version/python-$Version-embed-amd64.zip" $zip "python $Version"

# Un archivo que no empieza por PK no es un ZIP: casi siempre es la página de
# bloqueo de un antivirus devuelta con código 200.
$firma = [System.Text.Encoding]::ASCII.GetString((Get-Content $zip -Encoding Byte -TotalCount 2))
if ($firma -ne "PK") {
    Remove-Item $zip -Force
    throw "lo descargado no es un ZIP. Suele significar que un antivirus o proxy interceptó la conexión y devolvió su página de bloqueo."
}
Expand-Archive -Path $zip -DestinationPath $Destino -Force
} else {
    Write-Host "[1/4] intérprete ya presente, se omite la descarga"
}

Write-Host "[2/4] habilitando los import"
# La embeddable trae un archivo ._pth que recorta sys.path y, sobre todo, deja
# comentado "import site". Sin descomentarlo, pip no se puede ni instalar ni
# usar, porque site-packages no entra en la ruta de búsqueda.
$pth = Get-ChildItem $Destino -Filter "python*._pth" | Select-Object -First 1
if (-not $pth) { throw "no se encontró el archivo ._pth dentro del ZIP" }
$contenido = Get-Content $pth.FullName
$contenido = $contenido -replace '^#\s*import\s+site', 'import site'
if ($contenido -notcontains 'import site') { $contenido += 'import site' }
if ($contenido -notcontains 'Lib\site-packages') { $contenido += 'Lib\site-packages' }
$contenido | Set-Content $pth.FullName -Encoding ASCII
Write-Host "      $($pth.Name) ajustado"

Write-Host "[3/4] instalando pip"
$getpip = Join-Path $Temp "get-pip.py"
Bajar "https://bootstrap.pypa.io/get-pip.py" $getpip "get-pip"
& $Exe $getpip --no-warn-script-location 2>&1 | Select-Object -Last 2
if ($LASTEXITCODE -ne 0) { throw "falló la instalación de pip" }

Write-Host "[4/4] comprobando"
& $Exe --version
& $Exe -m pip --version

Remove-Item $Temp -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Python autocontenido en: $Destino"
Write-Host "No está en el PATH del sistema, y es deliberado: el agente lo invoca"
Write-Host "por ruta, así que no interfiere con nada más de la máquina."
Write-Host ""
