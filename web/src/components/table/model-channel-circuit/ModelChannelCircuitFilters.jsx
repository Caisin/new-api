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
import { Input, Select } from '@douyinfe/semi-ui';

const ModelChannelCircuitFilters = ({
  searchKeyword,
  setSearchKeyword,
  statusFilter,
  setStatusFilter,
  t,
}) => {
  return (
    <div className='flex flex-col md:flex-row gap-3 w-full'>
      <Input
        value={searchKeyword}
        onChange={setSearchKeyword}
        placeholder={t('搜索模型 ID')}
        showClear
      />
      <Select
        value={statusFilter}
        onChange={setStatusFilter}
        style={{ minWidth: 220 }}
        optionList={[
          { label: t('全部状态'), value: 'all' },
          { label: t('仅自动熔断'), value: 'auto_disabled' },
          { label: t('仅手动禁用'), value: 'manual_disabled' },
          { label: t('仅异常模型'), value: 'abnormal' },
        ]}
      />
    </div>
  );
};

export default ModelChannelCircuitFilters;
