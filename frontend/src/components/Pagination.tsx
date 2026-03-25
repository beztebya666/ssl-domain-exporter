type Props = {
  page: number
  totalPages: number
  onPageChange: (page: number) => void
  summary?: string
}

export default function Pagination({ page, totalPages, onPageChange, summary }: Props) {
  const safePage = Math.max(1, Math.min(page, totalPages))

  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
      <div className="text-xs text-slate-500">{summary}</div>
      <div className="flex items-center gap-2">
        <button
          className="btn-ghost border border-slate-700"
          disabled={safePage <= 1}
          onClick={() => onPageChange(Math.max(1, safePage - 1))}
        >
          Previous
        </button>
        <span className="text-sm text-slate-300">Page {safePage} / {totalPages}</span>
        <button
          className="btn-ghost border border-slate-700"
          disabled={safePage >= totalPages}
          onClick={() => onPageChange(Math.min(totalPages, safePage + 1))}
        >
          Next
        </button>
      </div>
    </div>
  )
}
