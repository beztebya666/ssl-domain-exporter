import type { InventorySourceDraft } from '../lib/domainSources'
import { buildSourceRef, formatSourceDisplayName } from '../lib/domainSources'

type Props = {
  draft: InventorySourceDraft
  onChange: (next: InventorySourceDraft) => void
}

export default function InventorySourceEditor({ draft, onChange }: Props) {
  const setDraft = (patch: Partial<InventorySourceDraft>) => onChange({ ...draft, ...patch })
  const generatedName = formatSourceDisplayName(draft.sourceType, buildSourceRef(draft))
  const isManual = draft.sourceType === 'manual'

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <label className="label">Source type</label>
          <select
            className="select"
            value={draft.sourceType}
            onChange={event => setDraft({ sourceType: event.target.value as InventorySourceDraft['sourceType'] })}
          >
            <option value="manual">Manual endpoint</option>
            <option value="kubernetes_secret">Kubernetes TLS secret</option>
            <option value="f5_certificate">F5 BIG-IP certificate</option>
          </select>
        </div>

        <div>
          <label className="label">{isManual ? 'Domain / host' : 'Inventory name'}</label>
          <input
            className="input"
            placeholder={isManual ? 'example.com' : generatedName || 'Optional custom inventory label'}
            value={draft.name}
            onChange={event => setDraft({ name: event.target.value })}
            required={isManual}
          />
          <p className="mt-1 text-xs text-slate-500">
            {isManual
              ? '`https://` and path fragments are removed automatically.'
              : 'Optional. Leave blank to use the generated source identifier shown below.'}
          </p>
        </div>
      </div>

      {draft.sourceType === 'kubernetes_secret' && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div>
            <label className="label">Namespace</label>
            <input
              className="input"
              placeholder="production"
              value={draft.k8sNamespace}
              onChange={event => setDraft({ k8sNamespace: event.target.value })}
              required
            />
          </div>
          <div>
            <label className="label">Secret name</label>
            <input
              className="input"
              placeholder="ingress-tls"
              value={draft.k8sSecretName}
              onChange={event => setDraft({ k8sSecretName: event.target.value })}
              required
            />
          </div>
          <div>
            <label className="label">Certificate serial</label>
            <input
              className="input font-mono text-xs"
              placeholder="Optional leaf serial pin"
              value={draft.k8sCertificateSerial}
              onChange={event => setDraft({ k8sCertificateSerial: event.target.value })}
            />
          </div>
        </div>
      )}

      {draft.sourceType === 'f5_certificate' && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div>
            <label className="label">Partition</label>
            <input
              className="input"
              placeholder="Common"
              value={draft.f5Partition}
              onChange={event => setDraft({ f5Partition: event.target.value })}
              required
            />
          </div>
          <div>
            <label className="label">Certificate name</label>
            <input
              className="input"
              placeholder="wildcard-example-com"
              value={draft.f5CertificateName}
              onChange={event => setDraft({ f5CertificateName: event.target.value })}
              required
            />
          </div>
          <div>
            <label className="label">Certificate serial</label>
            <input
              className="input font-mono text-xs"
              placeholder="Optional serial pin"
              value={draft.f5Serial}
              onChange={event => setDraft({ f5Serial: event.target.value })}
            />
          </div>
        </div>
      )}

      {!isManual && (
        <div className="rounded-xl border border-blue-500/15 bg-blue-500/5 px-4 py-3 text-xs text-slate-300">
          <div className="font-medium text-white">Source-backed monitoring</div>
          <div className="mt-1">
            This inventory item tracks the certificate directly from the configured source instead of resolving and probing a live HTTPS endpoint.
          </div>
          {generatedName && (
            <div className="mt-2 text-slate-400">
              Generated name preview: <span className="font-mono text-slate-200">{generatedName}</span>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
