import type { Domain, DomainWritePayload } from '../types'

export type InventorySourceType = Domain['source_type']

export type InventorySourceDraft = {
  name: string
  sourceType: InventorySourceType
  k8sNamespace: string
  k8sSecretName: string
  k8sCertificateSerial: string
  f5Partition: string
  f5CertificateName: string
  f5Serial: string
}

type SourceSeed = Pick<DomainWritePayload, 'name' | 'source_type' | 'source_ref'>

export function normalizeSourceType(sourceType?: string | null): InventorySourceType {
  switch ((sourceType ?? '').trim()) {
    case 'kubernetes_secret':
      return 'kubernetes_secret'
    case 'f5_certificate':
      return 'f5_certificate'
    default:
      return 'manual'
  }
}

export function isManualSource(sourceType?: string | null): boolean {
  return normalizeSourceType(sourceType) === 'manual'
}

export function createInventorySourceDraft(seed?: Partial<SourceSeed> | null): InventorySourceDraft {
  const sourceType = normalizeSourceType(seed?.source_type)
  const sourceRef = seed?.source_ref ?? {}

  return {
    name: (seed?.name ?? formatSourceDisplayName(sourceType, sourceRef)).trim(),
    sourceType,
    k8sNamespace: (sourceRef.namespace ?? '').trim(),
    k8sSecretName: (sourceRef.secret_name ?? '').trim(),
    k8sCertificateSerial: (sourceRef.certificate_serial ?? '').trim(),
    f5Partition: (sourceRef.partition ?? '').trim(),
    f5CertificateName: (sourceRef.certificate_name ?? '').trim(),
    f5Serial: (sourceRef.serial ?? '').trim(),
  }
}

export function buildSourceWritePayload(draft: InventorySourceDraft): Pick<DomainWritePayload, 'name' | 'source_type' | 'source_ref'> {
  const sourceType = normalizeSourceType(draft.sourceType)
  const name = draft.name.trim()

  if (sourceType === 'manual') {
    return {
      name,
      source_type: 'manual',
      source_ref: {},
    }
  }

  const sourceRef = buildSourceRef(draft)
  return {
    name: name || formatSourceDisplayName(sourceType, sourceRef),
    source_type: sourceType,
    source_ref: sourceRef,
  }
}

export function buildSourceRef(draft: InventorySourceDraft): Record<string, string> {
  if (draft.sourceType === 'kubernetes_secret') {
    const sourceRef: Record<string, string> = {
      namespace: draft.k8sNamespace.trim(),
      secret_name: draft.k8sSecretName.trim(),
    }
    if (draft.k8sCertificateSerial.trim()) {
      sourceRef.certificate_serial = draft.k8sCertificateSerial.trim()
    }
    return sourceRef
  }

  if (draft.sourceType === 'f5_certificate') {
    const sourceRef: Record<string, string> = {
      partition: draft.f5Partition.trim(),
      certificate_name: draft.f5CertificateName.trim(),
    }
    if (draft.f5Serial.trim()) {
      sourceRef.serial = draft.f5Serial.trim()
    }
    return sourceRef
  }

  return {}
}

export function formatSourceDisplayName(sourceType?: string | null, sourceRef?: Record<string, string> | null): string {
  const normalized = normalizeSourceType(sourceType)
  const ref = sourceRef ?? {}

  if (normalized === 'kubernetes_secret') {
    const namespace = (ref.namespace ?? '').trim()
    const secretName = (ref.secret_name ?? '').trim()
    if (namespace && secretName) {
      return `k8s:${namespace}/${secretName}`
    }
  }

  if (normalized === 'f5_certificate') {
    const partition = (ref.partition ?? '').trim()
    const certificateName = (ref.certificate_name ?? '').trim()
    if (partition && certificateName) {
      return `f5:${partition}/${certificateName}`
    }
  }

  return ''
}

export function formatSourceSummary(sourceType?: string | null, sourceRef?: Record<string, string> | null): string {
  const normalized = normalizeSourceType(sourceType)
  const ref = sourceRef ?? {}

  if (normalized === 'kubernetes_secret') {
    const namespace = (ref.namespace ?? '').trim()
    const secretName = (ref.secret_name ?? '').trim()
    const serial = (ref.certificate_serial ?? '').trim()
    const base = [namespace, secretName].filter(Boolean).join('/')
    if (base && serial) return `${base} | serial ${serial}`
    return base || serial
  }

  if (normalized === 'f5_certificate') {
    const partition = (ref.partition ?? '').trim()
    const certificateName = (ref.certificate_name ?? '').trim()
    const serial = (ref.serial ?? '').trim()
    const base = [partition, certificateName].filter(Boolean).join('/')
    if (base && serial) return `${base} | serial ${serial}`
    return base || serial
  }

  return ''
}

export function sourceTypeLabel(sourceType?: string | null): string {
  switch (normalizeSourceType(sourceType)) {
    case 'kubernetes_secret':
      return 'Kubernetes Secret'
    case 'f5_certificate':
      return 'F5 Certificate'
    default:
      return 'Manual Endpoint'
  }
}

export function sourceTypeBadge(sourceType?: string | null): string {
  switch (normalizeSourceType(sourceType)) {
    case 'kubernetes_secret':
      return 'K8S'
    case 'f5_certificate':
      return 'F5'
    default:
      return 'Manual'
  }
}
