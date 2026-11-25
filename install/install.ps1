$ErrorActionPreference = 'Stop'
$productId = $env:FRACTURE_STORE_PRODUCT_ID
if ($productId -notmatch '^[A-Z0-9]{12}$') {
    throw 'The Microsoft Store listing is not published yet. Do not install an older Inno build for Windows notification capture.'
}
Start-Process "ms-windows-store://pdp/?ProductId=$productId"
