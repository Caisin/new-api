/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useMemo } from 'react';
import { Button, Space, Tag, Typography } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import { timestamp2string } from '../../../helpers';

const { Text } = Typography;

const renderTimestamp = (timestamp, t) => {
  if (!timestamp) return <Text type='secondary'>-</Text>;
  return <Text>{timestamp2string(timestamp)}</Text>;
};

const renderStatusTag = (status, t) => {
  switch (status) {
    case 'enabled':
      return <Tag color='green'>{t('已启用')}</Tag>;
    case 'auto_disabled':
      return <Tag color='orange'>{t('自动熔断')}</Tag>;
    case 'manual_disabled':
      return <Tag color='red'>{t('手动禁用')}</Tag>;
    default:
      return <Tag>{status || '-'}</Tag>;
  }
};

const renderManualTag = (enabled, t) => {
  return enabled ? (
    <Tag color='green'>{t('策略启用')}</Tag>
  ) : (
    <Tag color='red'>{t('策略禁用')}</Tag>
  );
};

const ModelChannelCircuitDetailTable = ({
  channels,
  detailLoading,
  moveDraftChannel,
  toggleDraftManualEnabled,
  runChannelAction,
  actionLoadingKey,
  t,
}) => {
  const columns = useMemo(
    () => [
      {
        title: t('顺位'),
        dataIndex: 'order',
        render: (_, __, index) => <Tag color='blue'>#{index + 1}</Tag>,
      },
      {
        title: t('渠道'),
        dataIndex: 'channel_name',
        render: (_, record) => (
          <div className='flex flex-col'>
            <Space wrap>
              <span>{record.channel_name || `#${record.channel_id}`}</span>
              {record.channel_missing ? (
                <Tag color='red'>{t('渠道已删除')}</Tag>
              ) : null}
            </Space>
            <Text type='secondary' size='small'>
              {t('渠道 ID')}: {record.channel_id}
            </Text>
          </div>
        ),
      },
      {
        title: t('优先级'),
        dataIndex: 'priority',
      },
      {
        title: t('策略状态'),
        dataIndex: 'manual_enabled',
        render: (value) => renderManualTag(value, t),
      },
      {
        title: t('运行状态'),
        dataIndex: 'status',
        render: (value) => renderStatusTag(value, t),
      },
      {
        title: t('失败次数'),
        dataIndex: 'consecutive_failures',
        render: (value) => value || 0,
      },
      {
        title: t('原因'),
        dataIndex: 'disable_reason',
        render: (value) => value || '-',
      },
      {
        title: t('下次探测'),
        dataIndex: 'probe_after_at',
        render: (value) => renderTimestamp(value, t),
      },
      {
        title: t('最近失败'),
        dataIndex: 'last_failure_at',
        render: (value) => renderTimestamp(value, t),
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        render: (_, record, index) => (
          <Space wrap>
            <Button
              size='small'
              disabled={index === 0}
              onClick={() => moveDraftChannel(record.channel_id, 'up')}
            >
              {t('上移')}
            </Button>
            <Button
              size='small'
              disabled={index === channels.length - 1}
              onClick={() => moveDraftChannel(record.channel_id, 'down')}
            >
              {t('下移')}
            </Button>
            <Button
              size='small'
              type='tertiary'
              disabled={record.channel_missing}
              onClick={() => toggleDraftManualEnabled(record.channel_id)}
            >
              {record.manual_enabled ? t('设为策略禁用') : t('设为策略启用')}
            </Button>
            <Button
              size='small'
              disabled={record.channel_missing}
              loading={actionLoadingKey === `enable:${record.channel_id}`}
              onClick={() => runChannelAction(record.channel_id, 'enable')}
            >
              {t('启用')}
            </Button>
            <Button
              size='small'
              type='danger'
              disabled={record.channel_missing}
              loading={actionLoadingKey === `disable:${record.channel_id}`}
              onClick={() => runChannelAction(record.channel_id, 'disable')}
            >
              {t('禁用')}
            </Button>
            <Button
              size='small'
              theme='solid'
              type='primary'
              disabled={record.channel_missing}
              loading={actionLoadingKey === `probe:${record.channel_id}`}
              onClick={() => runChannelAction(record.channel_id, 'probe')}
            >
              {t('真实探测')}
            </Button>
          </Space>
        ),
      },
    ],
    [
      actionLoadingKey,
      channels.length,
      moveDraftChannel,
      runChannelAction,
      t,
      toggleDraftManualEnabled,
    ],
  );

  return (
    <CardTable
      rowKey='channel_id'
      columns={columns}
      dataSource={channels}
      loading={detailLoading}
      hidePagination={true}
      scroll={{ x: 'max-content' }}
      size='middle'
    />
  );
};

export default ModelChannelCircuitDetailTable;
