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

import React from 'react';
import { Button, Empty, SideSheet, Space, Typography } from '@douyinfe/semi-ui';
import { IconClose } from '@douyinfe/semi-icons';
import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import ModelChannelCircuitDetailTable from '../ModelChannelCircuitDetailTable';

const { Text, Title } = Typography;

const EditModelChannelPolicyModal = ({
  visible,
  onClose,
  detail,
  channels,
  detailLoading,
  saving,
  hasUnsavedChanges,
  refreshCurrentDetail,
  resetDraft,
  savePolicies,
  moveDraftChannel,
  toggleDraftManualEnabled,
  runChannelAction,
  actionLoadingKey,
  t,
}) => {
  const isMobile = useIsMobile();

  return (
    <SideSheet
      placement='right'
      visible={visible}
      width={isMobile ? '100%' : 1100}
      onCancel={onClose}
      title={
        <div className='flex flex-col'>
          <Title heading={5} style={{ margin: 0 }}>
            {t('模型渠道熔断详情')}
          </Title>
          <Text type='secondary'>
            {detail?.model
              ? `${t('模型 ID')}: ${detail.model}`
              : t('加载中...')}
          </Text>
        </div>
      }
      closeIcon={
        <Button
          className='semi-button-tertiary semi-button-size-small semi-button-borderless'
          type='button'
          icon={<IconClose />}
          onClick={onClose}
        />
      }
      bodyStyle={{ padding: 16 }}
      footer={
        <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3 w-full'>
          <Text type='secondary'>
            {hasUnsavedChanges
              ? t('当前有未保存的排序或策略状态变更')
              : t('当前草稿与服务端一致')}
          </Text>
          <Space wrap>
            <Button onClick={refreshCurrentDetail} loading={detailLoading}>
              {t('刷新')}
            </Button>
            <Button onClick={resetDraft} disabled={!hasUnsavedChanges}>
              {t('重置草稿')}
            </Button>
            <Button
              theme='solid'
              type='primary'
              onClick={savePolicies}
              loading={saving}
              disabled={!hasUnsavedChanges}
            >
              {t('保存排序')}
            </Button>
          </Space>
        </div>
      }
    >
      {detailLoading || detail?.channels?.length ? (
        <ModelChannelCircuitDetailTable
          channels={channels}
          detailLoading={detailLoading}
          moveDraftChannel={moveDraftChannel}
          toggleDraftManualEnabled={toggleDraftManualEnabled}
          runChannelAction={runChannelAction}
          actionLoadingKey={actionLoadingKey}
          t={t}
        />
      ) : (
        <Empty
          description={t('当前模型暂无渠道策略')}
          style={{ padding: 48 }}
        />
      )}
    </SideSheet>
  );
};

export default EditModelChannelPolicyModal;
