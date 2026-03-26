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
import { Button, Space, Typography } from '@douyinfe/semi-ui';

const { Text } = Typography;

const ModelChannelCircuitActions = ({
  loading,
  refresh,
  selectedModel,
  openDetail,
  t,
}) => {
  return (
    <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3 w-full'>
      <Space wrap>
        <Button onClick={refresh} loading={loading}>
          {t('刷新')}
        </Button>
        {selectedModel ? (
          <Button
            type='primary'
            theme='solid'
            onClick={() => openDetail(selectedModel)}
          >
            {t('继续编辑')}
          </Button>
        ) : null}
      </Space>
      <Text type='secondary'>
        {selectedModel
          ? t('当前选中模型：{{model}}', { model: selectedModel })
          : t('选择一个模型后可查看并编辑渠道优先级与熔断状态')}
      </Text>
    </div>
  );
};

export default ModelChannelCircuitActions;
