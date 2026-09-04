# nvidia-uwp-patch

Parchea la base de datos de perfiles de NVIDIA App (`fingerprint.db`) para que reconozca juegos **UWP / Microsoft Store** que NVIDIA no detecta de forma nativa.

## El problema

NVIDIA App mantiene una base de datos XML (`fingerprint.db`) que asocia juegos con su plataforma (Steam, Epic, GOG…). Los juegos UWP (Microsoft Store / Xbox PC) no aparecen, así que NVIDIA App no les aplica perfiles gráficos, no los lista, y no permite optimizarlos.

Esta herramienta localiza esa base de datos, la parchea con las entradas que faltan, y hace backup antes de tocar nada.

## Requisitos

- Windows 10/11
- **NVIDIA App** instalada (la versión moderna, no GeForce Experience)
- **Windows Terminal** o **PowerShell 7** — no uses CMD. El programa imprime caracteres Unicode (✓ ⊘ ✗) que CMD no renderiza.

## Uso

### Parchear todo (modo por defecto)

```powershell
.\nvidia-uwp-patch.exe
```

```
Processing: C:\Users\TuUsuario\AppData\Local\NVIDIA Corporation\NVIDIA App\NvBackend\ApplicationOntology\data\fingerprint.db
  ✓ added uwp version(s) of "final_fantasy_vii_remake"
  ⊘ fingerprint "starfield" already has uwp version(s)
  ✗ fingerprint "nonexistent_game" not found in database
```

Cada símbolo significa:
- **✓** — parcheado (versión añadida o actualizada)
- **⊘** — ya estaba bien, nada que hacer
- **✗** — no se pudo (fingerprint no encontrada, o sin versión fuente para UWP)

### Ver qué se cambiaría sin tocar nada

```powershell
.\nvidia-uwp-patch.exe --dry-run
```

Mismo output pero sin escribir a disco. Útil para verificar antes de ejecutar de verdad.

### Listar los juegos del manifiesto

```powershell
.\nvidia-uwp-patch.exe --list
```

```
Games database version: 1
Total games: 3

  final_fantasy_vii_remake
    AppUserModelId: 39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping
    UWPPackageFamilyName: 39EA002F.EXED1_n746a19ndrrjg
```

### Parchear un solo juego

```powershell
.\nvidia-uwp-patch.exe --game final_fantasy_vii_remake
```

Si el fingerprint no existe en el manifiesto, sale con error (exit code ≠ 0).

### Usar un manifiesto local propio

```powershell
.\nvidia-uwp-patch.exe --games-json .\mi-lista.json
```

Ignora el remoto y el caché — usa exclusivamente tu fichero. Si el fichero es inválido, falla explícitamente.

## El manifiesto (`games.json`)

Define qué juegos parchear y cómo. El programa lo descarga automáticamente desde este repo (con fallback a copia embebida y caché local).

### Formato

```json
{
  "version": 1,
  "games": [
    {
      "fingerprint": "final_fantasy_vii_remake",
      "app_user_model_id": "39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping",
      "versions": ["uwp"]
    }
  ]
}
```

### Campos

| Campo | Tipo | Descripción |
|---|---|---|
| `fingerprint` | string | Nombre exacto de la entrada en `fingerprint.db` (minúsculas, guiones bajos) |
| `app_user_model_id` | string | El AppUserModelID de la app UWP: `PackageFamilyName!AppId` |
| `versions` | []string | Versiones a asegurar: `"uwp"` (crear si falta) y/o `"steam"`, `"epic"`, etc. (actualizar si existen) |
| `overrides` | map | Campos XML a sobreescribir o añadir en la versión |
| `remove` | []string | Campos XML a eliminar de la versión |

### Ejemplo con overrides y removals

```json
{
  "fingerprint": "some_game",
  "app_user_model_id": "SomePkg_abc123!AppGame",
  "versions": ["uwp", "steam"],
  "overrides": {
    "DriverProfile": "SomeGame_UWP.exe"
  },
  "remove": ["WhisperModePopsFactor"]
}
```

Esto:
1. Si `uwp` no existe → la crea a partir de la versión Steam (o la primera no-UWP), con los overrides y removals aplicados
2. Si `steam` existe → la actualiza con los mismos overrides y removals
3. Si `uwp` ya existe sin overrides ni removals → no hace nada (idempotente)

### ¿Cómo encuentro el `fingerprint` y el `app_user_model_id`?

**Fingerprint:** busca en `fingerprint.db` (abre con un editor de texto) el nombre del juego que quieres parchear. Es el atributo `name` del elemento `<Fingerprint>`.

**AppUserModelID:** en PowerShell:

```powershell
Get-StartApps | Where-Object { $_.Name -like "*nombre del juego*" }
```

Te da algo como `39EA002F.EXED1_n746a19ndrrjg!AppFINALFANTASYVIIREMAKEShipping` — eso es el ID completo.

## Qué hace exactamente

Para cada juego con `versions: ["uwp"]`:

1. Busca la fingerprint en `fingerprint.db`
2. Busca la mejor versión fuente (prioridad: Steam > primera no-UWP)
3. Crea una versión `uwp` nueva:
   - Elimina campos específicos de otras tiendas (SteamAppIds, EpicAppId, Files, Launch…)
   - Añade `Distributor: UWP`, `UWPPackageFamilyName`, `AppUserModelId`
   - Aplica tus overrides y removals
4. Hace backup de `fingerprint.db` → `fingerprint.db.bak` (solo la primera vez, no sobreescribe)
5. Escribe la DB parcheada

## Resolución del manifiesto

El programa busca `games.json` en este orden:

1. **Remoto** (GitHub) — si hay red, descarga la última versión y la cachea
2. **Caché local** (`%LOCALAPPDATA%\nvidia-uwp-patch\games.json`) — si no hay red pero hay caché de una descarga anterior
3. **Embebido** en el .exe — fallback final, siempre disponible

Si la caché existe pero está corrupta, te avisa y cae al embebido.

## Build

```bash
make build
```

Genera `nvidia-uwp-patch.exe` para Windows amd64.

## Licencia

Uso personal. Modifica y redistribuye libremente.
