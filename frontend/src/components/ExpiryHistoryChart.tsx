import { useMemo, useState } from 'react'
import { formatDays } from '../lib/formatDays'

type Point = {
  time: string
  ssl: number | null | undefined
  domain: number | null | undefined
}

type Props = {
  data: Point[]
  showDomain: boolean
}

type SeriesPoint = {
  x: number
  y: number
  value: number
}

const VIEW_WIDTH = 720
const VIEW_HEIGHT = 220
const MARGIN = { top: 16, right: 18, bottom: 34, left: 42 }
const GRID_LINES = 4

export default function ExpiryHistoryChart({ data, showDomain }: Props) {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)

  const chart = useMemo(() => buildChartModel(data, showDomain), [data, showDomain])

  if (data.length === 0) {
    return (
      <div className="flex h-[220px] items-center justify-center rounded-xl border border-slate-800 bg-slate-950/40 text-sm text-slate-500">
        No history points yet.
      </div>
    )
  }

  const activeIndex = hoveredIndex != null ? hoveredIndex : data.length - 1
  const activePoint = data[Math.max(0, Math.min(activeIndex, data.length - 1))]
  const activeX = chart.xForIndex(activeIndex)

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3 text-xs">
        <LegendSwatch color="var(--chart-ssl)" label="SSL expiry (days)" />
        {showDomain && <LegendSwatch color="var(--chart-domain)" label="Domain expiry (days)" />}
      </div>

      <div className="relative h-[220px] w-full overflow-hidden rounded-xl border border-slate-800 bg-slate-950/30 p-2">
        <svg viewBox={`0 0 ${VIEW_WIDTH} ${VIEW_HEIGHT}`} className="h-full w-full" aria-label="Expiry history chart" role="img">
          {chart.yTicks.map((tick) => (
            <g key={`grid-${tick.value}`}>
              <line
                x1={MARGIN.left}
                y1={tick.y}
                x2={VIEW_WIDTH - MARGIN.right}
                y2={tick.y}
                stroke="var(--chart-grid)"
                strokeDasharray="4 4"
              />
              <text
                x={MARGIN.left - 10}
                y={tick.y + 4}
                textAnchor="end"
                fontSize="11"
                fill="var(--chart-axis)"
              >
                {tick.value}d
              </text>
            </g>
          ))}

          {chart.xTicks.map((tick) => (
            <text
              key={`x-${tick.index}`}
              x={chart.xForIndex(tick.index)}
              y={VIEW_HEIGHT - 10}
              textAnchor="middle"
              fontSize="11"
              fill="var(--chart-axis)"
            >
              {tick.label}
            </text>
          ))}

          {chart.sslPath && (
            <path
              d={chart.sslPath}
              fill="none"
              stroke="var(--chart-ssl)"
              strokeWidth="2.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          )}

          {showDomain && chart.domainPath && (
            <path
              d={chart.domainPath}
              fill="none"
              stroke="var(--chart-domain)"
              strokeWidth="2.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          )}

          <line
            x1={activeX}
            y1={MARGIN.top}
            x2={activeX}
            y2={VIEW_HEIGHT - MARGIN.bottom}
            stroke="var(--chart-axis)"
            strokeDasharray="3 5"
            opacity="0.5"
          />

          {renderSeriesMarker(chart.sslPoints[activeIndex], 'var(--chart-ssl)')}
          {showDomain ? renderSeriesMarker(chart.domainPoints[activeIndex], 'var(--chart-domain)') : null}

          {data.map((point, index) => {
            const previousX = index === 0 ? MARGIN.left : chart.xForIndex(index - 1)
            const nextX = index === data.length - 1 ? VIEW_WIDTH - MARGIN.right : chart.xForIndex(index + 1)
            const startX = index === 0 ? MARGIN.left : (previousX + chart.xForIndex(index)) / 2
            const width = index === data.length - 1
              ? (VIEW_WIDTH - MARGIN.right) - startX
              : ((nextX + chart.xForIndex(index)) / 2) - startX

            return (
              <rect
                key={`hit-${point.time}-${index}`}
                x={startX}
                y={MARGIN.top}
                width={Math.max(width, 8)}
                height={(VIEW_HEIGHT - MARGIN.bottom) - MARGIN.top}
                fill="transparent"
                onMouseEnter={() => setHoveredIndex(index)}
                onMouseMove={() => setHoveredIndex(index)}
                onMouseLeave={() => setHoveredIndex(null)}
              />
            )
          })}
        </svg>

        <div className="pointer-events-none absolute right-4 top-4 max-w-[220px] rounded-xl border border-slate-700 bg-slate-950/95 px-3 py-2 text-xs shadow-2xl">
          <div className="font-medium text-slate-100">{activePoint.time}</div>
          <div className="mt-1 flex items-center justify-between gap-4 text-slate-300">
            <span>SSL</span>
            <span className="font-medium text-blue-300">
              {activePoint.ssl == null ? 'N/A' : formatDays(activePoint.ssl)}
            </span>
          </div>
          {showDomain && (
            <div className="mt-1 flex items-center justify-between gap-4 text-slate-300">
              <span>Domain</span>
              <span className="font-medium text-emerald-300">
                {activePoint.domain == null ? 'N/A' : formatDays(activePoint.domain)}
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function LegendSwatch({ color, label }: { color: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-slate-700 bg-slate-900/50 px-3 py-1 text-slate-300">
      <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: color }} />
      {label}
    </span>
  )
}

function renderSeriesMarker(point: SeriesPoint | undefined, color: string) {
  if (!point) return null
  return (
    <g>
      <circle cx={point.x} cy={point.y} r="6" fill={color} opacity="0.18" />
      <circle cx={point.x} cy={point.y} r="3.2" fill={color} />
    </g>
  )
}

function buildChartModel(data: Point[], showDomain: boolean) {
  const chartWidth = VIEW_WIDTH - MARGIN.left - MARGIN.right
  const chartHeight = VIEW_HEIGHT - MARGIN.top - MARGIN.bottom
  const numericValues = data.flatMap((point) => [
    point.ssl != null ? point.ssl : null,
    showDomain && point.domain != null ? point.domain : null,
  ].filter((value): value is number => value != null))
  const maxValue = numericValues.length > 0 ? Math.max(...numericValues) : 1
  const yMax = niceCeil(Math.max(1, maxValue))
  const xForIndex = (index: number) => MARGIN.left + ((data.length <= 1 ? 0 : index / (data.length - 1)) * chartWidth)
  const yForValue = (value: number) => MARGIN.top + chartHeight - ((value / yMax) * chartHeight)

  const sslPoints = data.map((point, index) =>
    point.ssl == null ? undefined : { x: xForIndex(index), y: yForValue(point.ssl), value: point.ssl },
  )
  const domainPoints = data.map((point, index) =>
    point.domain == null ? undefined : { x: xForIndex(index), y: yForValue(point.domain), value: point.domain },
  )

  const tickStep = yMax / GRID_LINES
  const yTicks = Array.from({ length: GRID_LINES + 1 }, (_, idx) => {
    const value = Math.round((GRID_LINES - idx) * tickStep)
    return { value, y: yForValue(value) }
  })

  const labelEvery = Math.max(1, Math.ceil(data.length / 6))
  const xTicks = data
    .map((point, index) => ({ label: point.time, index }))
    .filter((_, index) => index % labelEvery === 0 || index === data.length - 1)

  return {
    xForIndex,
    yTicks,
    xTicks,
    sslPoints,
    domainPoints,
    sslPath: buildLinePath(sslPoints),
    domainPath: buildLinePath(domainPoints),
  }
}

function buildLinePath(points: Array<SeriesPoint | undefined>): string {
  let path = ''
  let open = false
  points.forEach((point) => {
    if (!point) {
      open = false
      return
    }
    path += `${open ? 'L' : 'M'} ${point.x.toFixed(2)} ${point.y.toFixed(2)} `
    open = true
  })
  return path.trim()
}

function niceCeil(value: number): number {
  if (value <= 10) return Math.ceil(value)
  const magnitude = 10 ** Math.floor(Math.log10(value))
  const normalized = value / magnitude
  const niceBase = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10
  return niceBase * magnitude
}
