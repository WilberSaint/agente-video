# instalar-kokoro.ps1 — deja Kokoro listo en bin\kokoro\
#
# Kokoro es la voz local de mejor calidad: gratis, ilimitada y en CPU, con una
# entonación bastante más natural que Piper. Necesita Python, que es la razón
# por la que no viene de entrada; ejecuta antes .\instalar-python.ps1
#
#   .\instalar-kokoro.ps1

[CmdletBinding()]
param([switch]$Forzar)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Raiz    = $PSScriptRoot
$Python  = Join-Path $Raiz "bin\python\python.exe"
$Destino = Join-Path $Raiz "bin\kokoro"

if (-not (Test-Path $Python)) {
    throw "no se encontró $Python. Ejecuta primero: .\instalar-python.ps1"
}
New-Item -ItemType Directory -Path $Destino -Force | Out-Null

Write-Host ""
Write-Host "[1/2] dependencias de Python"
& $Python -m pip install --quiet --no-warn-script-location kokoro-onnx soundfile
if ($LASTEXITCODE -ne 0) { throw "falló la instalación de kokoro-onnx" }
Write-Host "      kokoro-onnx y soundfile instalados"

Write-Host "[2/2] modelo y voces"
$archivos = @(
    @{ n = "kokoro-v1.0.onnx"; u = "https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/kokoro-v1.0.onnx" },
    @{ n = "voices-v1.0.bin";  u = "https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/voices-v1.0.bin" }
)
foreach ($a in $archivos) {
    $ruta = Join-Path $Destino $a.n
    if ((Test-Path $ruta) -and -not $Forzar) {
        Write-Host ("      {0} ya está ({1} MB)" -f $a.n, [math]::Round((Get-Item $ruta).Length / 1MB, 1))
        continue
    }
    Write-Host "      bajando $($a.n)..." -NoNewline
    Invoke-WebRequest -Uri $a.u -OutFile $ruta -UseBasicParsing -MaximumRedirection 10
    Write-Host (" ok ({0} MB)" -f [math]::Round((Get-Item $ruta).Length / 1MB, 1))
}

Write-Host ""
Write-Host "Listo. En el perfil:"
Write-Host '    "voz": { "proveedor": "kokoro", "modelo": "em_alex", "procesar": true }'
Write-Host ""
Write-Host "Voces: em_alex, em_santa (masculinas), ef_dora (femenina)."
Write-Host ""
