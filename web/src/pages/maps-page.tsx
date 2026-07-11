import { useState, useEffect, useMemo } from "react"
import { Star, RefreshCw, Grid3x3, Plus, X, GripVertical } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import mapsData from "@/data/cs2-maps.json"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type MapMode = "comp" | "hostage" | "practice"

type MapInfo = {
  id: string
  name: string
  mode: MapMode
  thumbnail: string
}

type WorkshopMap = {
  workshopId: string
  name: string
  // Populated by the backend when a STEAM_API_KEY is configured; points at the
  // panel's own cached thumbnail endpoint. Absent when thumbnails are disabled.
  thumbnailUrl?: string
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const FAVORITES_KEY = "nokit_map_favorites"
const MAP_CYCLE_KEY = "nokit_map_cycle"

const MODE_COLORS: Record<MapMode, string> = {
  comp: "bg-green-500/10 text-green-600 border-green-500/20",
  hostage: "bg-blue-500/10 text-blue-600 border-blue-500/20",
  practice: "bg-amber-500/10 text-amber-600 border-amber-500/20",
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function loadFavorites(): string[] {
  try {
    const raw = localStorage.getItem(FAVORITES_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function saveFavorites(favorites: string[]) {
  try {
    localStorage.setItem(FAVORITES_KEY, JSON.stringify(favorites))
  } catch {
    // ignore
  }
}

function loadMapCycle(): string[] {
  try {
    const raw = localStorage.getItem(MAP_CYCLE_KEY)
    if (!raw) return ["de_mirage", "de_inferno", "de_nuke", "de_dust2", "de_anubis", "de_overpass", "de_ancient"]
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return ["de_mirage", "de_inferno", "de_nuke", "de_dust2", "de_anubis", "de_overpass", "de_ancient"]
  }
}

function saveMapCycle(cycle: string[]) {
  try {
    localStorage.setItem(MAP_CYCLE_KEY, JSON.stringify(cycle))
  } catch {
    // ignore
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

type MapsPageProps = {
  serverId: string
}

export function MapsPage({ serverId }: MapsPageProps) {
  const [currentMap, setCurrentMap] = useState("de_mirage")
  const [favorites, setFavorites] = useState<string[]>([])
  const [showFavsOnly, setShowFavsOnly] = useState(false)
  const [workshopId, setWorkshopId] = useState("")
  const [mapCycle, setMapCycle] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [workshopMaps, setWorkshopMaps] = useState<WorkshopMap[]>([])
  const [workshopLoading, setWorkshopLoading] = useState(false)
  const [workshopError, setWorkshopError] = useState<string | null>(null)

  const fetchWorkshopMaps = async () => {
    setWorkshopLoading(true)
    setWorkshopError(null)
    // Guard against the request hanging forever (e.g. an unresponsive RCON
    // connection) so the "Sync workshop" spinner always resolves.
    const controller = new AbortController()
    const timeout = setTimeout(() => controller.abort(), 25000)
    try {
      const res = await fetch(`/api/servers/${serverId}/maps/workshop`, {
        signal: controller.signal,
      })
      if (!res.ok) {
        throw new Error(`Server returned ${res.status}`)
      }
      const data: WorkshopMap[] = await res.json()
      setWorkshopMaps(data)
    } catch (err) {
      console.error("Failed to fetch workshop maps:", err)
      if (err instanceof DOMException && err.name === "AbortError") {
        setWorkshopError("Workshop sync timed out. The server may be unreachable or slow to respond via RCON.")
      } else {
        setWorkshopError("Could not load workshop maps. Make sure the server is connected via RCON.")
      }
    } finally {
      clearTimeout(timeout)
      setWorkshopLoading(false)
    }
  }

  useEffect(() => {
    setFavorites(loadFavorites())
    setMapCycle(loadMapCycle())
    fetchWorkshopMaps()
    // TODO: fetch current map from server via RCON `status` command
  }, [serverId])

  useEffect(() => {
    saveFavorites(favorites)
  }, [favorites])

  useEffect(() => {
    saveMapCycle(mapCycle)
  }, [mapCycle])

  const standardMaps = useMemo(() => mapsData.standard as MapInfo[], [])

  const displayedMaps = useMemo(() => {
    if (!showFavsOnly) return standardMaps
    return standardMaps.filter((m) => favorites.includes(m.id))
  }, [standardMaps, showFavsOnly, favorites])

  const favoriteMaps = useMemo(() => {
    return standardMaps.filter((m) => favorites.includes(m.id))
  }, [standardMaps, favorites])

  const toggleFavorite = (mapId: string) => {
    setFavorites((prev) =>
      prev.includes(mapId) ? prev.filter((id) => id !== mapId) : [...prev, mapId]
    )
  }

  const changeMap = async (mapId: string) => {
    setLoading(true)
    try {
      const res = await fetch(`/api/servers/${serverId}/rcon`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: `changelevel ${mapId}` }),
      })
      if (res.ok) {
        setCurrentMap(mapId)
      }
    } catch (err) {
      console.error("Failed to change map:", err)
    } finally {
      setLoading(false)
    }
  }

  const loadWorkshopMap = async () => {
    if (!workshopId.trim()) return
    setLoading(true)
    try {
      const res = await fetch(`/api/servers/${serverId}/rcon`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: `host_workshop_map ${workshopId}` }),
      })
      if (res.ok) {
        // Success feedback
      }
    } catch (err) {
      console.error("Failed to load workshop map:", err)
    } finally {
      setLoading(false)
      setWorkshopId("")
    }
  }

  // TODO: implement add to cycle from map cards
  // const addToMapCycle = (mapId: string) => {
  //   if (!mapCycle.includes(mapId)) {
  //     setMapCycle((prev) => [...prev, mapId])
  //   }
  // }

  const removeFromMapCycle = (index: number) => {
    setMapCycle((prev) => prev.filter((_, i) => i !== index))
  }

  // TODO: implement drag-and-drop reordering
  // const moveMapInCycle = (fromIndex: number, toIndex: number) => {
  //   setMapCycle((prev) => {
  //     const copy = [...prev]
  //     const [item] = copy.splice(fromIndex, 1)
  //     copy.splice(toIndex, 0, item)
  //     return copy
  //   })
  // }

  return (
    <div className="flex flex-1 flex-col gap-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Maps</h1>
          <p className="text-sm text-muted-foreground">
            {standardMaps.length} standard · {workshopMaps.length} workshop · current: {currentMap}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={fetchWorkshopMaps} disabled={workshopLoading}>
            <RefreshCw className={cn("mr-2 h-4 w-4", workshopLoading && "animate-spin")} />
            Sync workshop
          </Button>
          <Button variant="outline" size="sm">
            <Grid3x3 className="mr-2 h-4 w-4" />
            Browse collection
          </Button>
        </div>
      </div>

      {/* Favorites Row */}
      {favoriteMaps.length > 0 && (
        <div>
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
              Favorites
            </h2>
            <button
              onClick={() => setShowFavsOnly(!showFavsOnly)}
              className="flex items-center gap-2 text-xs text-muted-foreground hover:text-foreground"
            >
              <Star className={cn("h-3 w-3", showFavsOnly && "fill-amber-500 text-amber-500")} />
              show favs only
            </button>
          </div>
          <div className="flex gap-3 overflow-x-auto pb-2">
            {favoriteMaps.map((map) => (
              <button
                key={map.id}
                onClick={() => changeMap(map.id)}
                disabled={loading}
                className={cn(
                  "flex shrink-0 flex-col gap-2 rounded-lg border p-3 transition-all hover:border-primary",
                  currentMap === map.id && "border-green-600 bg-green-500/5"
                )}
              >
                <div className="h-16 w-24 rounded bg-muted" />
                <div className="text-left">
                  <div className="flex items-center gap-1 text-xs font-medium">
                    {map.name}
                    {currentMap === map.id && (
                      <span className="ml-1 rounded bg-green-600 px-1.5 py-0.5 text-[10px] font-semibold text-white">
                        active
                      </span>
                    )}
                  </div>
                  <div className={cn("mt-1 inline-block rounded border px-1.5 py-0.5 text-[10px]", MODE_COLORS[map.mode])}>
                    {map.mode}
                  </div>
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Standard Pool Grid */}
      <div>
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Standard Pool
        </h2>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {displayedMaps.map((map) => (
            <MapCard
              key={map.id}
              map={map}
              isActive={currentMap === map.id}
              isFavorite={favorites.includes(map.id)}
              onToggleFavorite={() => toggleFavorite(map.id)}
              onClick={() => changeMap(map.id)}
              disabled={loading}
            />
          ))}
        </div>
      </div>

      {/* Workshop Section */}
      <div>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
            Workshop
          </h2>
          <span className="text-xs text-muted-foreground">
            {workshopMaps.length} installed
          </span>
        </div>

        <div className="mb-4 flex gap-2">
          <Input
            placeholder="workshop ID — e.g. 3070900859"
            value={workshopId}
            onChange={(e) => setWorkshopId(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && loadWorkshopMap()}
          />
          <Button onClick={loadWorkshopMap} disabled={loading || !workshopId.trim()}>
            Download & switch
          </Button>
        </div>

        {workshopError && (
          <div className="mb-4 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {workshopError}
          </div>
        )}

        {workshopLoading ? (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <div
                key={i}
                className="flex flex-col overflow-hidden rounded-lg border border-border"
              >
                <div className="aspect-video w-full animate-pulse bg-muted" />
                <div className="flex flex-col gap-2 p-3">
                  <div className="h-4 w-2/3 animate-pulse rounded bg-muted" />
                  <div className="h-3 w-1/2 animate-pulse rounded bg-muted" />
                </div>
              </div>
            ))}
          </div>
        ) : workshopMaps.length === 0 && !workshopError ? (
          <div className="rounded-lg border border-dashed border-border py-12 text-center text-sm text-muted-foreground">
            No workshop maps found on this server.
            <br />
            Use the input above to download and switch to a workshop map by ID.
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {workshopMaps.map((map) => (
              <WorkshopMapCard
                key={map.workshopId}
                map={map}
                onSwitch={() => changeMap(`workshop/${map.workshopId}/${map.name}`)}
                disabled={loading}
              />
            ))}
          </div>
        )}
      </div>

      {/* Map Cycle Editor */}
      <div>
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Map Cycle
        </h2>
        <div className="rounded-lg border border-border bg-card p-4">
          <div className="mb-3 flex flex-wrap gap-2">
            {mapCycle.map((mapId, index) => {
              const map = standardMaps.find((m) => m.id === mapId)
              return (
                <div
                  key={`${mapId}-${index}`}
                  className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-1.5 text-sm"
                >
                  <button
                    className="cursor-grab text-muted-foreground hover:text-foreground"
                    title="Drag to reorder"
                  >
                    <GripVertical className="h-4 w-4" />
                  </button>
                  <span className="font-mono text-xs text-muted-foreground">
                    {String(index + 1).padStart(2, "0")}
                  </span>
                  <span className="font-medium">{map?.name || mapId}</span>
                  <button
                    onClick={() => removeFromMapCycle(index)}
                    className="text-muted-foreground hover:text-destructive"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              )
            })}
            <button
              className="flex items-center gap-1 rounded-md border border-dashed border-border px-3 py-1.5 text-sm text-muted-foreground hover:border-primary hover:text-primary"
            >
              <Plus className="h-3 w-3" />
              add
            </button>
          </div>
          <div className="flex justify-end">
            <Button variant="outline" size="sm">
              Save mapcycle.txt
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// MapCard Component
// ---------------------------------------------------------------------------

type MapCardProps = {
  map: MapInfo
  isActive: boolean
  isFavorite: boolean
  onToggleFavorite: () => void
  onClick: () => void
  disabled: boolean
}

function MapCard({
  map,
  isActive,
  isFavorite,
  onToggleFavorite,
  onClick,
  disabled,
}: MapCardProps) {
  return (
    <div
      className={cn(
        "group relative flex flex-col overflow-hidden rounded-lg border transition-all",
        isActive
          ? "border-green-600 bg-green-500/5 shadow-lg shadow-green-500/20"
          : "border-border hover:border-primary"
      )}
    >
      <button
        onClick={onClick}
        disabled={disabled}
        className="flex flex-col"
      >
        {/* Thumbnail placeholder */}
        <div className="aspect-video w-full bg-gradient-to-br from-zinc-800 to-zinc-900" />
        
        <div className="flex flex-col gap-2 p-3">
          <div className="flex items-center justify-between">
            <h3 className="font-semibold">{map.name}</h3>
            {isActive && (
              <span className="rounded bg-green-600 px-1.5 py-0.5 text-[10px] font-semibold text-white">
                ● active
              </span>
            )}
          </div>
          <div className={cn("inline-block self-start rounded border px-2 py-0.5 text-xs", MODE_COLORS[map.mode])}>
            {map.mode}
          </div>
        </div>
      </button>

      {/* Favorite star */}
      <button
        onClick={(e) => {
          e.stopPropagation()
          onToggleFavorite()
        }}
        className="absolute right-2 top-2 rounded-full bg-black/50 p-1.5 backdrop-blur-sm hover:bg-black/70"
      >
        <Star
          className={cn(
            "h-4 w-4 transition-colors",
            isFavorite ? "fill-amber-500 text-amber-500" : "text-white"
          )}
        />
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// WorkshopMapCard Component
// ---------------------------------------------------------------------------

type WorkshopMapCardProps = {
  map: WorkshopMap
  onSwitch?: () => void
  disabled?: boolean
}

function WorkshopMapCard({ map, onSwitch, disabled }: WorkshopMapCardProps) {
  const [thumbFailed, setThumbFailed] = useState(false)
  const showThumb = Boolean(map.thumbnailUrl) && !thumbFailed

  return (
    <div className="group flex flex-col overflow-hidden rounded-lg border border-border transition-all hover:border-primary">
      {/* Steam Workshop thumbnail, with a gradient fallback while loading or if
          Steam has no preview image / thumbnails are disabled. */}
      <div className="relative aspect-video w-full bg-gradient-to-br from-purple-900/20 to-blue-900/20">
        {showThumb && (
          <img
            src={map.thumbnailUrl}
            alt={map.name}
            loading="lazy"
            onError={() => setThumbFailed(true)}
            className="absolute inset-0 h-full w-full object-cover"
          />
        )}
      </div>

      <div className="flex flex-col gap-2 p-3">
        <div className="flex items-center justify-between">
          <h3 className="truncate font-semibold" title={map.name}>
            {map.name}
          </h3>
          <Star className="h-4 w-4 shrink-0 text-muted-foreground" />
        </div>
        <div className="font-mono text-xs text-muted-foreground">
          #{map.workshopId}
        </div>
        <Button
          size="sm"
          variant="outline"
          className="mt-1 w-full text-xs"
          onClick={onSwitch}
          disabled={disabled}
        >
          Switch to map
        </Button>
      </div>
    </div>
  )
}
