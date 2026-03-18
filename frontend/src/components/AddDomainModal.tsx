import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { X, Plus, Upload, FolderPlus } from 'lucide-react'
import { createDomain, fetchFolders, createFolder } from '../api/client'

interface Props {
  onClose: () => void
}

const INTERVALS = [
  { label: '1 hour', value: 3600 },
  { label: '3 hours', value: 10800 },
  { label: '6 hours', value: 21600 },
  { label: '12 hours', value: 43200 },
  { label: '24 hours', value: 86400 },
]

export default function AddDomainModal({ onClose }: Props) {
  const [name, setName] = useState('')
  const [tags, setTags] = useState('')
  const [interval, setInterval] = useState(21600)
  const [port, setPort] = useState(443)
  const [folderValue, setFolderValue] = useState<string>('')
  const [bulkMode, setBulkMode] = useState(false)
  const [bulkText, setBulkText] = useState('')
  const [customCAPEM, setCustomCAPEM] = useState('')
  const [submitError, setSubmitError] = useState('')
  const [newFolderName, setNewFolderName] = useState('')

  const qc = useQueryClient()
  const { data: folders = [] } = useQuery({ queryKey: ['folders'], queryFn: fetchFolders })

  const mutation = useMutation({
    mutationFn: createDomain,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['domains'] })
      qc.invalidateQueries({ queryKey: ['summary'] })
    },
  })

  const folderMutation = useMutation({
    mutationFn: createFolder,
    onSuccess: (folder) => {
      qc.invalidateQueries({ queryKey: ['folders'] })
      setFolderValue(String(folder.id))
      setNewFolderName('')
    },
  })

  const selectedFolderID = folderValue ? Number(folderValue) : undefined

  const handleSingle = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setSubmitError('')

    try {
      await mutation.mutateAsync({
        name: name.trim(),
        tags,
        check_interval: interval,
        port,
        folder_id: selectedFolderID,
        custom_ca_pem: customCAPEM.trim() || undefined,
      })
      onClose()
    } catch (err) {
      setSubmitError(extractErrorMessage(err))
    }
  }

  const handleBulk = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitError('')

    const domains = bulkText
      .split('\n')
      .map(l => l.trim())
      .filter(l => l.length > 0 && !l.startsWith('#'))

    try {
      for (const d of domains) {
        await mutation.mutateAsync({
          name: d,
          tags,
          check_interval: interval,
          port,
          folder_id: selectedFolderID,
          custom_ca_pem: customCAPEM.trim() || undefined,
        })
      }
      onClose()
    } catch (err) {
      setSubmitError(extractErrorMessage(err))
    }
  }

  const loadCertFile = async (file: File | null) => {
    if (!file) return
    try {
      const content = await file.text()
      setCustomCAPEM(content)
    } catch {
      // ignore read errors for now
    }
  }

  const handleCreateFolder = async () => {
    const name = newFolderName.trim()
    if (!name) return
    setSubmitError('')
    try {
      await folderMutation.mutateAsync(name)
    } catch (err) {
      setSubmitError(extractErrorMessage(err))
    }
  }

  return (
    <div className="fixed inset-0 bg-black/75 flex items-center justify-center z-50 p-4">
      <div className="bg-gray-900 border border-gray-700 rounded-2xl w-full max-w-3xl shadow-2xl">
        <div className="flex items-center justify-between p-5 border-b border-gray-800">
          <h2 className="font-semibold text-white">Add Domain(s)</h2>
          <button onClick={onClose} className="btn-ghost p-1.5 rounded-lg">
            <X size={16} />
          </button>
        </div>

        <div className="p-5">
          <div className="flex gap-2 mb-5">
            <button
              className={`btn text-xs flex-1 ${!bulkMode ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
              onClick={() => setBulkMode(false)}
            >
              Single domain
            </button>
            <button
              className={`btn text-xs flex-1 ${bulkMode ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
              onClick={() => setBulkMode(true)}
            >
              Bulk import
            </button>
          </div>

          <form onSubmit={bulkMode ? handleBulk : handleSingle} className="space-y-4">
            {bulkMode ? (
              <div>
                <label className="label">Domains (one per line)</label>
                <textarea
                  className="input h-28 resize-none font-mono"
                  placeholder="example.com&#10;another.org&#10;# comments are ignored"
                  value={bulkText}
                  onChange={e => setBulkText(e.target.value)}
                  required
                />
              </div>
            ) : (
              <div>
                <label className="label">Domain name</label>
                <input
                  className="input"
                  placeholder="example.com"
                  value={name}
                  onChange={e => setName(e.target.value)}
                  required
                />
                <p className="text-xs text-gray-500 mt-1">`https://` or path parts are removed automatically.</p>
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="label">Tags (optional)</label>
                <input
                  className="input"
                  placeholder="production, web, api"
                  value={tags}
                  onChange={e => setTags(e.target.value)}
                />
              </div>

              <div>
                <label className="label">Check interval</label>
                <select
                  className="input"
                  value={interval}
                  onChange={e => setInterval(Number(e.target.value))}
                >
                  {INTERVALS.map(i => (
                    <option key={i.value} value={i.value}>{i.label}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="label">HTTPS Port</label>
                <input
                  className="input"
                  type="number"
                  min={1}
                  max={65535}
                  value={port}
                  onChange={e => setPort(Math.max(1, Math.min(65535, Number(e.target.value) || 443)))}
                />
              </div>

              <div>
                <label className="label">Folder</label>
                <select className="input" value={folderValue} onChange={e => setFolderValue(e.target.value)}>
                  <option value="">No folder</option>
                  {folders.map(folder => (
                    <option key={folder.id} value={folder.id}>{folder.name}</option>
                  ))}
                </select>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-[1fr_auto] gap-2 items-end">
              <div>
                <label className="label">Create folder (optional)</label>
                <input
                  className="input"
                  placeholder="backend, public-sites, staging"
                  value={newFolderName}
                  onChange={e => setNewFolderName(e.target.value)}
                />
              </div>
              <button type="button" className="btn-ghost border border-gray-700" onClick={handleCreateFolder} disabled={folderMutation.isPending}>
                <FolderPlus size={14} />
                {folderMutation.isPending ? 'Creating...' : 'Create'}
              </button>
            </div>

            <div>
              <div className="flex items-center justify-between mb-1.5">
                <label className="label mb-0">Custom Root CA (optional)</label>
                <label className="btn-ghost border border-gray-700 px-2 py-1 text-xs cursor-pointer inline-flex items-center gap-1.5">
                  <Upload size={12} />
                  Load .crt/.pem
                  <input
                    type="file"
                    accept=".crt,.pem,.cer,text/plain"
                    className="hidden"
                    onChange={e => loadCertFile(e.target.files?.[0] ?? null)}
                  />
                </label>
              </div>
              <textarea
                className="input h-36 resize-y font-mono text-xs"
                placeholder="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
                value={customCAPEM}
                onChange={e => setCustomCAPEM(e.target.value)}
              />
              <p className="text-xs text-gray-500 mt-1">Used as additional trust root for SSL/HTTPS checks for this domain.</p>
            </div>

            {submitError && (
              <div className="text-xs text-amber-300 bg-amber-500/10 border border-amber-600/20 rounded-lg px-3 py-2">
                {submitError}
              </div>
            )}

            <div className="flex gap-3 pt-2">
              <button type="button" onClick={onClose} className="btn-ghost flex-1 border border-gray-700">
                Cancel
              </button>
              <button
                type="submit"
                className="btn-primary flex-1"
                disabled={mutation.isPending}
              >
                <Plus size={15} />
                {mutation.isPending ? 'Adding...' : bulkMode ? 'Import All' : 'Add Domain'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}

function extractErrorMessage(err: unknown): string {
  if (axios.isAxiosError(err)) {
    const status = err.response?.status
    const backendMessage = (err.response?.data as { error?: string } | undefined)?.error

    if (status === 409) {
      return backendMessage || 'Domain already exists.'
    }
    if (status === 401) {
      return 'Unauthorized. Please log in first.'
    }
    if (backendMessage) {
      return backendMessage
    }
    if (err.message) {
      return err.message
    }
  }

  return 'Failed to add domain.'
}
