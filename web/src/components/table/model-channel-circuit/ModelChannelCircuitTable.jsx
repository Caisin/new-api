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
import { Button, Empty, Space, Tag } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import CardTable from '../../common/ui/CardTable';

const renderCountTag = (count, color) => (
  <Tag color={count > 0 ? color : 'grey'} shape='circle'>
    {count}
  </Tag>
);

const ModelChannelCircuitTable = ({
  models,
  loading,
  openDetail,
  selectedModel,
  t,
}) => {
  const columns = useMemo(
    () => [
      {
        title: t('模型 ID'),
        dataIndex: 'model',
        render: (text, record) => (
          <Space>
            <span>{text}</span>
            {selectedModel === record.model ? (
              <Tag color='blue' shape='circle'>
                {t('编辑中')}
              </Tag>
            ) : null}
          </Space>
        ),
      },
      {
        title: t('渠道数'),
        dataIndex: 'policy_count',
      },
      {
        title: t('自动熔断'),
        dataIndex: 'auto_disabled_count',
        render: (value) => renderCountTag(value, 'orange'),
      },
      {
        title: t('手动禁用'),
        dataIndex: 'manual_disabled_count',
        render: (value) => renderCountTag(value, 'red'),
      },
      {
        title: t('操作'),
        dataIndex: 'operate',
        render: (_, record) => (
          <Button
            size='small'
            type='primary'
            theme='solid'
            onClick={() => openDetail(record.model)}
          >
            {t('查看详情')}
          </Button>
        ),
      },
    ],
    [openDetail, selectedModel, t],
  );

  return (
    <CardTable
      rowKey='model'
      columns={columns}
      dataSource={models}
      loading={loading}
      hidePagination={true}
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('暂无模型渠道策略')}
          style={{ padding: 30 }}
        />
      }
      onRow={(record) => ({
        onClick: () => openDetail(record.model),
        style: { cursor: 'pointer' },
      })}
      className='rounded-xl overflow-hidden'
      size='middle'
    />
  );
};

export default ModelChannelCircuitTable;
