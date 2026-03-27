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
import { Typography } from '@douyinfe/semi-ui';
import CardPro from '../../common/ui/CardPro';
import { useModelChannelCircuitData } from '../../../hooks/model-channel-circuit/useModelChannelCircuitData';
import ModelChannelCircuitActions from './ModelChannelCircuitActions';
import ModelChannelCircuitFilters from './ModelChannelCircuitFilters';
import ModelChannelCircuitTable from './ModelChannelCircuitTable';
import EditModelChannelPolicyModal from './modals/EditModelChannelPolicyModal';

const { Text } = Typography;

const ModelChannelCircuitPage = () => {
  const circuitData = useModelChannelCircuitData();

  return (
    <>
      <EditModelChannelPolicyModal
        visible={circuitData.drawerVisible}
        onClose={circuitData.closeDetail}
        detail={circuitData.detail}
        channels={circuitData.draftChannels}
        detailLoading={circuitData.detailLoading}
        saving={circuitData.saving}
        hasUnsavedChanges={circuitData.hasUnsavedChanges}
        canSavePolicies={circuitData.canSavePolicies}
        refreshCurrentDetail={circuitData.refreshCurrentDetail}
        resetDraft={circuitData.resetDraft}
        savePolicies={circuitData.savePolicies}
        moveDraftChannel={circuitData.moveDraftChannel}
        reorderDraftChannel={circuitData.reorderDraftChannel}
        toggleDraftManualEnabled={circuitData.toggleDraftManualEnabled}
        runChannelAction={circuitData.runChannelAction}
        actionLoadingKey={circuitData.actionLoadingKey}
        t={circuitData.t}
      />

      <CardPro
        type='type3'
        descriptionArea={
          <Text type='secondary'>
            {circuitData.t(
              '按模型维度维护渠道优先级、手动启停状态和自动熔断恢复情况。',
            )}
          </Text>
        }
        actionsArea={<ModelChannelCircuitActions {...circuitData} />}
        searchArea={<ModelChannelCircuitFilters {...circuitData} />}
        t={circuitData.t}
      >
        <ModelChannelCircuitTable
          models={circuitData.filteredModels}
          loading={circuitData.loading}
          openDetail={circuitData.openDetail}
          selectedModel={circuitData.selectedModel}
          t={circuitData.t}
        />
      </CardPro>
    </>
  );
};

export default ModelChannelCircuitPage;
