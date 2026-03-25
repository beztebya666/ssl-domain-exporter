import { useRef } from 'react'
import type { ReactNode } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'

type Props<T> = {
  items: T[]
  estimateSize?: number
  overscan?: number
  maxHeightClassName?: string
  itemKey: (item: T, index: number) => string | number
  renderItem: (item: T, index: number) => ReactNode
}

export default function VirtualizedCardList<T>({
  items,
  estimateSize = 108,
  overscan = 8,
  maxHeightClassName = 'max-h-[72vh]',
  itemKey,
  renderItem,
}: Props<T>) {
  const parentRef = useRef<HTMLDivElement | null>(null)

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateSize,
    overscan,
    getItemKey: (index) => itemKey(items[index], index),
  })

  const virtualItems = virtualizer.getVirtualItems()

  return (
    <div ref={parentRef} className={`overflow-auto pr-1 ${maxHeightClassName}`}>
      <div className="relative" style={{ height: `${virtualizer.getTotalSize()}px` }}>
        {virtualItems.map(virtualItem => (
          <div
            key={virtualItem.key}
            className="absolute left-0 top-0 w-full"
            style={{ transform: `translateY(${virtualItem.start}px)` }}
          >
            <div className="pb-2">
              {renderItem(items[virtualItem.index], virtualItem.index)}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
