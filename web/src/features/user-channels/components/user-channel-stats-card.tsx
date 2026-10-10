import { useQuery } from '@tanstack/react-query'
import { Coins, Flame, KeyRound, Radio } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatQuota } from '@/lib/format'
import { formatCurrencyFromUSD } from '@/lib/currency'

import { getUserChannelStats, type UserChannelStats } from '../api'

export function UserChannelStatsCard() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['user-channel-stats'],
    queryFn: async () => {
      try {
        const res = await getUserChannelStats()
        return res?.data
      } catch {
        return null
      }
    },
    retry: false,
    staleTime: 30 * 1000,
  })

  if (isLoading) {
    return (
      <div className='mb-4 grid grid-cols-2 divide-x rounded-lg border sm:grid-cols-4'>
        {['channels', 'active', 'quota', 'earnings'].map((key) => (
          <div key={key} className='min-w-0 px-2.5 py-2.5 sm:px-5 sm:py-4'>
            <Skeleton className='h-3.5 w-full' />
            <Skeleton className='mt-2 h-6 w-full sm:h-7' />
            <Skeleton className='mt-1.5 hidden h-3.5 w-24 md:block' />
          </div>
        ))}
      </div>
    )
  }

  const statsData: UserChannelStats = data || {
    total_channels: 0,
    enabled_channels: 0,
    total_used_quota: 0,
    total_earned_quota: 0,
    share_ratio: 0.8,
  }

  // Quota to USD is quota / 500000
  const earnedUSD = statsData.total_earned_quota / 500000

  const stats: {
    label: string
    value: string
    description: string
    icon: typeof KeyRound
    tone: IconBadgeTone
  }[] = [
    {
      label: t('Contributed Keys', '已共享 API Key'),
      value: statsData.total_channels.toString(),
      description: t('Total idle keys shared', '累计上传的闲置 Key 数量'),
      icon: KeyRound,
      tone: 'info',
    },
    {
      label: t('Active Channels', '在线服务渠道'),
      value: `${statsData.enabled_channels} / ${statsData.total_channels}`,
      description: t('Channels in service', '正常运行中的渠道'),
      icon: Radio,
      tone: 'success',
    },
    {
      label: t('Shared Traffic Quota', '共享流量消耗'),
      value: formatQuota(statsData.total_used_quota),
      description: t('Total quota consumed by others', '他人调用累计消耗配额'),
      icon: Flame,
      tone: 'chart-4',
    },
    {
      label: t('Earned Balance Reward', '累计分成收益'),
      value: `${formatCurrencyFromUSD(earnedUSD)} (${formatQuota(statsData.total_earned_quota)})`,
      description: t('80% revenue credited to your balance', '80% 收益已实时计入平台余额'),
      icon: Coins,
      tone: 'warning',
    },
  ]

  return (
    <div className='mb-4 grid grid-cols-2 divide-x rounded-lg border sm:grid-cols-4'>
      {stats.map((item) => {
        const Icon = item.icon
        return (
          <div
            key={item.label}
            className='min-w-0 px-2.5 py-2.5 sm:px-5 sm:py-4'
          >
            <div className='flex items-center justify-between gap-1'>
              <span className='truncate text-xs font-medium text-muted-foreground sm:text-sm'>
                {item.label}
              </span>
              <IconBadge tone={item.tone} size='sm' className='hidden sm:flex'>
                <Icon className='size-3.5' />
              </IconBadge>
            </div>
            <div className='mt-1 truncate text-lg font-semibold tracking-tight sm:mt-2 sm:text-2xl'>
              {item.value}
            </div>
            <div className='mt-0.5 truncate text-xs text-muted-foreground sm:mt-1'>
              {item.description}
            </div>
          </div>
        )
      })}
    </div>
  )
}
