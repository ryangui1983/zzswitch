/*
Copyright (C) 2023-2026 QuantumNous
*/
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { getChannels } from '@/features/channels/api'
import { CHANNEL_TYPES } from '@/features/channels/constants'
import type { Channel } from '@/features/channels/types'

const PREFERRED_TYPE_ORDER = [1, 48, 3, 8, 20, 14, 24]

const TYPE_LABELS: Record<number, string> = {
  1: 'OpenAI',
  3: 'Azure',
  48: 'xAI / Grok',
  8: 'Custom',
  20: 'OpenRouter',
  14: 'Anthropic',
  24: 'Gemini',
}

function parseChannelIds(raw: string): number[] {
  if (!raw.trim()) return []
  const ids = new Set<number>()
  for (const part of raw.split(/[,;\s]+/)) {
    const id = Number(part)
    if (Number.isInteger(id) && id > 0) ids.add(id)
  }
  return [...ids].sort((a, b) => a - b)
}

function serializeChannelIds(ids: number[]): string {
  return [...new Set(ids.filter((id) => id > 0))].sort((a, b) => a - b).join(',')
}

type CacheHitBoostChannelPickerProps = {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}

export function CacheHitBoostChannelPicker({
  value,
  onChange,
  disabled,
}: CacheHitBoostChannelPickerProps) {
  const { t } = useTranslation()
  const selected = useMemo(() => new Set(parseChannelIds(value)), [value])

  const channelsQuery = useQuery({
    queryKey: ['quota-cache-hit-boost-channels'],
    queryFn: async () => {
      const res = await getChannels({ p: 0, page_size: 1000, id_sort: true })
      return res.data?.items ?? []
    },
    staleTime: 60_000,
  })

  const groups = useMemo(() => {
    const items = channelsQuery.data ?? []
    const byType = new Map<number, Channel[]>()
    for (const ch of items) {
      const list = byType.get(ch.type) ?? []
      list.push(ch)
      byType.set(ch.type, list)
    }
    for (const list of byType.values()) {
      list.sort((a, b) => a.id - b.id)
    }
    const types = [...byType.keys()].sort((a, b) => {
      const ai = PREFERRED_TYPE_ORDER.indexOf(a)
      const bi = PREFERRED_TYPE_ORDER.indexOf(b)
      if (ai !== -1 || bi !== -1) {
        return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
      }
      return a - b
    })
    return types.map((type) => ({
      type,
      label: TYPE_LABELS[type] ?? (CHANNEL_TYPES as Record<number, string>)[type] ?? 'Unknown',
      channels: byType.get(type) ?? [],
    }))
  }, [channelsQuery.data])

  const toggle = (id: number, next: boolean) => {
    const set = new Set(selected)
    if (next) set.add(id)
    else set.delete(id)
    onChange(serializeChannelIds([...set]))
  }

  const toggleGroup = (channels: Channel[], next: boolean) => {
    const set = new Set(selected)
    for (const ch of channels) {
      if (next) set.add(ch.id)
      else set.delete(ch.id)
    }
    onChange(serializeChannelIds([...set]))
  }

  if (channelsQuery.isLoading) {
    return (
      <div className='text-muted-foreground text-sm'>{t('Loading...')}</div>
    )
  }

  if (channelsQuery.isError) {
    return (
      <div className='text-destructive text-sm'>
        {t('Failed to load channels')}
      </div>
    )
  }

  return (
    <div className='border-border max-h-96 space-y-4 overflow-y-auto rounded-md border p-3'>
      {groups.length === 0 ? (
        <div className='text-muted-foreground text-sm'>{t('No channels')}</div>
      ) : (
        groups.map((group) => {
          const selectedInGroup = group.channels.filter((ch) =>
            selected.has(ch.id)
          ).length
          const allSelected =
            group.channels.length > 0 && selectedInGroup === group.channels.length
          const someSelected = selectedInGroup > 0 && !allSelected
          return (
            <div key={group.type} className='space-y-2'>
              <label className='flex items-center gap-2 text-sm font-medium'>
                <Checkbox
                  checked={someSelected ? 'indeterminate' : allSelected}
                  disabled={disabled}
                  onCheckedChange={(v) =>
                    toggleGroup(group.channels, v === true)
                  }
                />
                <span>
                  {t(group.label)}
                  <span className='text-muted-foreground ml-2 font-normal'>
                    {selectedInGroup}/{group.channels.length}
                  </span>
                </span>
              </label>
              <div className='ml-6 grid gap-1 sm:grid-cols-2'>
                {group.channels.map((ch) => (
                  <label
                    key={ch.id}
                    className='flex items-center gap-2 text-sm'
                  >
                    <Checkbox
                      checked={selected.has(ch.id)}
                      disabled={disabled}
                      onCheckedChange={(v) => toggle(ch.id, v === true)}
                    />
                    <span className='truncate'>
                      #{ch.id} {ch.name}
                    </span>
                  </label>
                ))}
              </div>
            </div>
          )
        })
      )}
    </div>
  )
}
