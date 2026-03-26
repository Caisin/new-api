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

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../helpers';

const normalizeDraftChannels = (channels = []) =>
  (channels || []).map((channel, index, list) => ({
    ...channel,
    priority: (list.length - index) * 10,
  }));

const buildDraftSignature = (channels = []) =>
  normalizeDraftChannels(channels).map((channel) => ({
    channel_id: channel.channel_id,
    manual_enabled: channel.manual_enabled,
    priority: channel.priority,
  }));

export const useModelChannelCircuitData = () => {
  const { t } = useTranslation();
  const [models, setModels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [selectedModel, setSelectedModel] = useState('');
  const [detail, setDetail] = useState(null);
  const [draftChannels, setDraftChannels] = useState([]);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [actionLoadingKey, setActionLoadingKey] = useState('');

  const loadModels = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/model_channel_circuit/models');
      const { success, message, data } = res.data;
      if (!success) {
        showError(message || t('获取模型列表失败'));
        setModels([]);
        return;
      }
      const items = Array.isArray(data) ? data : [];
      setModels(
        items.map((item) => ({
          ...item,
          key: item.model,
        })),
      );
    } catch (error) {
      showError(t('获取模型列表失败'));
      setModels([]);
    } finally {
      setLoading(false);
    }
  }, [t]);

  const loadDetail = useCallback(
    async (modelName, { silent = false } = {}) => {
      if (!modelName) return null;
      if (!silent) {
        setDetailLoading(true);
      }
      try {
        const res = await API.get(
          `/api/model_channel_circuit/models/${encodeURIComponent(modelName)}`,
        );
        const { success, message, data } = res.data;
        if (!success) {
          showError(message || t('获取模型详情失败'));
          return null;
        }
        const nextDetail = {
          ...data,
          channels: (data?.channels || []).map((item) => ({
            ...item,
            key: item.channel_id,
          })),
        };
        setDetail(nextDetail);
        setDraftChannels(normalizeDraftChannels(nextDetail.channels || []));
        return nextDetail;
      } catch (error) {
        showError(t('获取模型详情失败'));
        return null;
      } finally {
        if (!silent) {
          setDetailLoading(false);
        }
      }
    },
    [t],
  );

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  const openDetail = useCallback(
    async (modelName) => {
      setSelectedModel(modelName);
      setDrawerVisible(true);
      await loadDetail(modelName);
    },
    [loadDetail],
  );

  const closeDetail = useCallback(() => {
    setDrawerVisible(false);
  }, []);

  const refreshCurrentDetail = useCallback(async () => {
    if (!selectedModel) return;
    await loadDetail(selectedModel);
  }, [loadDetail, selectedModel]);

  const filteredModels = useMemo(() => {
    const keyword = searchKeyword.trim().toLowerCase();
    return models.filter((item) => {
      const matchKeyword =
        keyword.length === 0 || item.model.toLowerCase().includes(keyword);
      if (!matchKeyword) {
        return false;
      }
      switch (statusFilter) {
        case 'auto_disabled':
          return item.auto_disabled_count > 0;
        case 'manual_disabled':
          return item.manual_disabled_count > 0;
        case 'abnormal':
          return item.auto_disabled_count > 0 || item.manual_disabled_count > 0;
        default:
          return true;
      }
    });
  }, [models, searchKeyword, statusFilter]);

  const hasUnsavedChanges = useMemo(() => {
    const saved = JSON.stringify(buildDraftSignature(detail?.channels || []));
    const draft = JSON.stringify(buildDraftSignature(draftChannels || []));
    return saved !== draft;
  }, [detail?.channels, draftChannels]);

  const moveDraftChannel = useCallback((channelId, direction) => {
    setDraftChannels((current) => {
      const next = [...current];
      const index = next.findIndex((item) => item.channel_id === channelId);
      if (index < 0) return current;
      const targetIndex = direction === 'up' ? index - 1 : index + 1;
      if (targetIndex < 0 || targetIndex >= next.length) return current;
      const temp = next[index];
      next[index] = next[targetIndex];
      next[targetIndex] = temp;
      return normalizeDraftChannels(next);
    });
  }, []);

  const toggleDraftManualEnabled = useCallback((channelId) => {
    setDraftChannels((current) =>
      normalizeDraftChannels(
        current.map((item) =>
          item.channel_id === channelId
            ? { ...item, manual_enabled: !item.manual_enabled }
            : item,
        ),
      ),
    );
  }, []);

  const resetDraft = useCallback(() => {
    setDraftChannels(normalizeDraftChannels(detail?.channels || []));
  }, [detail?.channels]);

  const savePolicies = useCallback(async () => {
    if (!selectedModel) return;
    setSaving(true);
    try {
      const payload = {
        channels: normalizeDraftChannels(draftChannels).map((item) => ({
          channel_id: item.channel_id,
          priority: item.priority,
          manual_enabled: item.manual_enabled,
        })),
      };
      const res = await API.put(
        `/api/model_channel_circuit/models/${encodeURIComponent(selectedModel)}/policies`,
        payload,
      );
      const { success, message } = res.data;
      if (!success) {
        showError(message || t('保存失败'));
        return false;
      }
      showSuccess(t('保存成功'));
      await Promise.all([
        loadModels(),
        loadDetail(selectedModel, { silent: true }),
      ]);
      return true;
    } catch (error) {
      showError(t('保存失败'));
      return false;
    } finally {
      setSaving(false);
    }
  }, [draftChannels, loadDetail, loadModels, selectedModel, t]);

  const runChannelAction = useCallback(
    async (channelId, action) => {
      if (!selectedModel || !channelId) return false;
      const key = `${action}:${channelId}`;
      setActionLoadingKey(key);
      try {
        const encodedModel = encodeURIComponent(selectedModel);
        let res;
        if (action === 'probe') {
          res = await API.post(
            `/api/model_channel_circuit/models/${encodedModel}/channel/${channelId}/probe`,
          );
          if (!res.data.success) {
            showError(res.data.message || t('真实探测失败'));
            return false;
          }
          showSuccess(res.data.message || t('真实探测成功'));
        } else {
          res = await API.post(
            `/api/model_channel_circuit/models/${encodedModel}/channel/${channelId}/${action}`,
          );
          const { success, message } = res.data;
          if (!success) {
            showError(message || t('操作失败'));
            return false;
          }
          showSuccess(action === 'enable' ? t('启用成功') : t('禁用成功'));
        }
        await Promise.all([
          loadModels(),
          loadDetail(selectedModel, { silent: true }),
        ]);
        return true;
      } catch (error) {
        showError(action === 'probe' ? t('真实探测失败') : t('操作失败'));
        return false;
      } finally {
        setActionLoadingKey('');
      }
    },
    [loadDetail, loadModels, selectedModel, t],
  );

  return {
    t,
    models,
    filteredModels,
    loading,
    detail,
    detailLoading,
    saving,
    drawerVisible,
    selectedModel,
    draftChannels,
    searchKeyword,
    statusFilter,
    actionLoadingKey,
    hasUnsavedChanges,
    setSearchKeyword,
    setStatusFilter,
    openDetail,
    closeDetail,
    refresh: loadModels,
    refreshCurrentDetail,
    moveDraftChannel,
    toggleDraftManualEnabled,
    resetDraft,
    savePolicies,
    runChannelAction,
  };
};

export default useModelChannelCircuitData;
