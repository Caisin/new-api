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

import React, { useCallback, useMemo, useState } from 'react';
import { Button, Space, Tag, Tooltip, Typography } from '@douyinfe/semi-ui';
import { GripVertical } from 'lucide-react';
import CardTable from '../../common/ui/CardTable';
import { timestamp2string } from '../../../helpers';

const { Text } = Typography;

const renderTimestamp = (timestamp, t) => {
  if (!timestamp) return <Text type='secondary'>-</Text>;
  return <Text>{timestamp2string(timestamp)}</Text>;
};

const renderStatusTag = (status, reason, t) => {
  let tag = <Tag>{status || '-'}</Tag>;
  switch (status) {
    case 'enabled':
      tag = <Tag color='green'>{t('已启用')}</Tag>;
      break;
    case 'auto_disabled':
      tag = <Tag color='orange'>{t('自动熔断')}</Tag>;
      break;
    case 'manual_disabled':
      tag = <Tag color='red'>{t('手动禁用')}</Tag>;
      break;
    default:
      break;
  }

  if (!reason) {
    return tag;
  }

  return (
    <Tooltip
      content={
        <div style={{ maxWidth: 320, wordBreak: 'break-all' }}>
          {t('原因')}: {reason}
        </div>
      }
    >
      <span>{tag}</span>
    </Tooltip>
  );
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
  reorderDraftChannel,
  toggleDraftManualEnabled,
  runChannelAction,
  actionLoadingKey,
  t,
}) => {
  const [draggedChannelId, setDraggedChannelId] = useState(0);
  const [dragOverChannelId, setDragOverChannelId] = useState(0);
  const [dragOverPosition, setDragOverPosition] = useState('before');

  const resetDragState = useCallback(() => {
    setDraggedChannelId(0);
    setDragOverChannelId(0);
    setDragOverPosition('before');
  }, []);

  const handleDragStart = useCallback((event, channelId) => {
    setDraggedChannelId(channelId);
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', String(channelId));
  }, []);

  const handleDragOver = useCallback(
    (event, channelId) => {
      event.preventDefault();
      if (!draggedChannelId || draggedChannelId === channelId) {
        return;
      }
      const rect = event.currentTarget.getBoundingClientRect();
      const position =
        event.clientY - rect.top > rect.height / 2 ? 'after' : 'before';
      setDragOverChannelId(channelId);
      setDragOverPosition(position);
      event.dataTransfer.dropEffect = 'move';
    },
    [draggedChannelId],
  );

  const handleDrop = useCallback(
    (event, channelId) => {
      event.preventDefault();
      const sourceChannelId = Number(
        draggedChannelId || event.dataTransfer.getData('text/plain'),
      );
      const position =
        dragOverChannelId === channelId ? dragOverPosition : 'before';
      reorderDraftChannel(sourceChannelId, channelId, position);
      resetDragState();
    },
    [
      dragOverChannelId,
      dragOverPosition,
      draggedChannelId,
      reorderDraftChannel,
      resetDragState,
    ],
  );

  const columns = useMemo(
    () => [
      {
        title: t('顺位'),
        dataIndex: 'order',
        render: (_, record, index) => (
          <div className='flex items-center gap-2'>
            <Tag color='blue'>#{index + 1}</Tag>
            <span
              className='inline-flex cursor-grab text-[var(--semi-color-text-2)]'
              title={t('拖动排序')}
            >
              <GripVertical size={16} />
            </span>
          </div>
        ),
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
        render: (value, record) =>
          renderStatusTag(value, record.disable_reason, t),
      },
      {
        title: t('失败次数'),
        dataIndex: 'consecutive_failures',
        render: (value) => value || 0,
      },
      {
        title: t('最近失败'),
        dataIndex: 'last_failure_at',
        render: (value, record) => {
          const timestampNode = renderTimestamp(value, t);
          if (!record.probe_after_at) {
            return timestampNode;
          }
          return (
            <Tooltip
              content={
                <div style={{ lineHeight: 1.6 }}>
                  <div>
                    {t('最近失败')}: {value ? timestamp2string(value) : '-'}
                  </div>
                  <div>
                    {t('下次探测')}: {timestamp2string(record.probe_after_at)}
                  </div>
                </div>
              }
            >
              <span>{timestampNode}</span>
            </Tooltip>
          );
        },
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        render: (_, record) => (
          <Space wrap>
            <Button
              size='small'
              type='tertiary'
              disabled={record.channel_missing}
              onClick={() => toggleDraftManualEnabled(record.channel_id)}
            >
              {record.manual_enabled ? t('关策略') : t('开策略')}
            </Button>
            <Button
              size='small'
              type={record.status === 'manual_disabled' ? 'primary' : 'danger'}
              theme={record.status === 'manual_disabled' ? 'solid' : 'light'}
              disabled={record.channel_missing}
              loading={
                actionLoadingKey === `enable:${record.channel_id}` ||
                actionLoadingKey === `disable:${record.channel_id}`
              }
              onClick={() =>
                runChannelAction(
                  record.channel_id,
                  record.status === 'manual_disabled' ? 'enable' : 'disable',
                )
              }
            >
              {record.status === 'manual_disabled' ? t('启用') : t('禁用')}
            </Button>
            <Button
              size='small'
              theme='solid'
              type='primary'
              disabled={record.channel_missing}
              loading={actionLoadingKey === `probe:${record.channel_id}`}
              onClick={() => runChannelAction(record.channel_id, 'probe')}
            >
              {t('探测')}
            </Button>
          </Space>
        ),
      },
    ],
    [
      actionLoadingKey,
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
      onRow={(record) => {
        const isDragging = record.channel_id === draggedChannelId;
        const isDropTarget =
          record.channel_id === dragOverChannelId &&
          draggedChannelId &&
          draggedChannelId !== record.channel_id;
        return {
          draggable: channels.length > 1,
          onDragStart: (event) => handleDragStart(event, record.channel_id),
          onDragOver: (event) => handleDragOver(event, record.channel_id),
          onDrop: (event) => handleDrop(event, record.channel_id),
          onDragEnd: resetDragState,
          style: {
            cursor: channels.length > 1 ? 'grab' : 'default',
            opacity: isDragging ? 0.55 : 1,
            boxShadow: isDropTarget
              ? `inset 0 ${
                  dragOverPosition === 'before' ? '3px 0 0 0' : '-3px 0 0 0'
                } var(--semi-color-primary)`
              : undefined,
          },
        };
      }}
    />
  );
};

export default ModelChannelCircuitDetailTable;
