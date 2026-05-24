/**
 * Vertex JSON 凭证导入弹窗。
 *
 * Vertex 不是 OAuth 授权流程，但它原本和 OAuth 登录页放在同一个入口中，
 * 用于上传 Google Cloud service account key JSON 并由 CPA 写入 auth-dir。
 * 将它抽成弹窗后，配额管理页可以保留完整的“登录/导入凭证”能力，
 * 同时避免用户为了导入 Vertex 凭证离开当前配额视图。
 */

import { useCallback, useRef, useState, type ChangeEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { useNotificationStore } from '@/stores';
import { vertexApi, type VertexImportResponse } from '@/services/api/vertex';
import iconVertex from '@/assets/icons/vertex.svg';
import styles from '@/pages/QuotaPage.module.scss';

interface VertexImportModalProps {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void | Promise<void>;
}

interface VertexImportResult {
  projectId?: string;
  email?: string;
  location?: string;
  authFile?: string;
}

interface VertexImportState {
  file?: File;
  fileName: string;
  location: string;
  loading: boolean;
  error?: string;
  result?: VertexImportResult;
}

const initialVertexState: VertexImportState = {
  fileName: '',
  location: '',
  loading: false,
};
const VERTEX_IMPORT_FILE_INPUT_ID = 'vertex-import-file-input';

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error) return error.message;
  if (typeof error === 'string') return error;
  if (error && typeof error === 'object' && 'message' in error) {
    const message = (error as { message?: unknown }).message;
    return typeof message === 'string' ? message : '';
  }
  return '';
};

export function VertexImportModal({ open, onClose, onSuccess }: VertexImportModalProps) {
  const { t } = useTranslation();
  const showNotification = useNotificationStore((state) => state.showNotification);
  const [state, setState] = useState<VertexImportState>(initialVertexState);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const handleFilePick = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleFileChange = useCallback((event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    if (!file.name.endsWith('.json')) {
      showNotification(t('vertex_import.file_required'), 'warning');
      event.target.value = '';
      return;
    }

    setState((prev) => ({
      ...prev,
      file,
      fileName: file.name,
      error: undefined,
      result: undefined,
    }));
    event.target.value = '';
  }, [showNotification, t]);

  const handleImport = useCallback(async () => {
    if (!state.file) {
      const message = t('vertex_import.file_required');
      setState((prev) => ({ ...prev, error: message }));
      showNotification(message, 'warning');
      return;
    }

    const location = state.location.trim();
    setState((prev) => ({ ...prev, loading: true, error: undefined, result: undefined }));
    try {
      const res: VertexImportResponse = await vertexApi.importCredential(
        state.file,
        location || undefined
      );
      const result: VertexImportResult = {
        projectId: res.project_id,
        email: res.email,
        location: res.location,
        authFile: res['auth-file'] ?? res.auth_file,
      };
      setState((prev) => ({ ...prev, loading: false, result }));
      showNotification(t('vertex_import.success'), 'success');
      void onSuccess?.();
    } catch (err: unknown) {
      const message = getErrorMessage(err);
      setState((prev) => ({
        ...prev,
        loading: false,
        error: message || t('notification.upload_failed'),
      }));
      const notification = message
        ? `${t('notification.upload_failed')}: ${message}`
        : t('notification.upload_failed');
      showNotification(notification, 'error');
    }
  }, [onSuccess, showNotification, state.file, state.location, t]);

  const handleClose = useCallback(() => {
    if (!state.loading) {
      setState(initialVertexState);
      onClose();
    }
  }, [onClose, state.loading]);

  return (
    <Modal
      open={open}
      title={
        <span className={styles.oauthModalTitle}>
          <img src={iconVertex} alt="" className={styles.oauthModalTitleIcon} />
          {t('vertex_import.title')}
        </span>
      }
      onClose={handleClose}
      closeDisabled={state.loading}
      width={680}
    >
      <div className={styles.oauthModalContent}>
        <div className={styles.oauthModalHint}>{t('vertex_import.description')}</div>
        <div className={styles.oauthModalField}>
          <Input
            label={t('vertex_import.location_label')}
            hint={t('vertex_import.location_hint')}
            value={state.location}
            onChange={(event) =>
              setState((prev) => ({
                ...prev,
                location: event.target.value,
              }))
            }
            placeholder={t('vertex_import.location_placeholder')}
          />
        </div>
        <div className={styles.vertexFileField}>
          <label className={styles.vertexFileLabel} htmlFor={VERTEX_IMPORT_FILE_INPUT_ID}>
            {t('vertex_import.file_label')}
          </label>
          <div className={styles.vertexFilePicker}>
            <Button variant="secondary" size="sm" onClick={handleFilePick} disabled={state.loading}>
              {t('vertex_import.choose_file')}
            </Button>
            <div
              className={`${styles.vertexFileName} ${
                state.fileName ? '' : styles.vertexFileNamePlaceholder
              }`.trim()}
            >
              {state.fileName || t('vertex_import.file_placeholder')}
            </div>
          </div>
          <div className={styles.oauthModalHint}>{t('vertex_import.file_hint')}</div>
          <input
            id={VERTEX_IMPORT_FILE_INPUT_ID}
            ref={fileInputRef}
            type="file"
            accept=".json,application/json"
            style={{ display: 'none' }}
            onChange={handleFileChange}
          />
        </div>
        <div className={styles.oauthModalActions}>
          <Button onClick={() => void handleImport()} loading={state.loading}>
            {t('vertex_import.import_button')}
          </Button>
        </div>
        {state.error && <div className="status-badge error">{state.error}</div>}
        {state.result && (
          <div className={styles.vertexResultBox}>
            <div className={styles.vertexResultTitle}>{t('vertex_import.result_title')}</div>
            <div className={styles.vertexResultList}>
              {state.result.projectId && (
                <div className={styles.vertexResultItem}>
                  <span>{t('vertex_import.result_project')}</span>
                  <strong>{state.result.projectId}</strong>
                </div>
              )}
              {state.result.email && (
                <div className={styles.vertexResultItem}>
                  <span>{t('vertex_import.result_email')}</span>
                  <strong>{state.result.email}</strong>
                </div>
              )}
              {state.result.location && (
                <div className={styles.vertexResultItem}>
                  <span>{t('vertex_import.result_location')}</span>
                  <strong>{state.result.location}</strong>
                </div>
              )}
              {state.result.authFile && (
                <div className={styles.vertexResultItem}>
                  <span>{t('vertex_import.result_file')}</span>
                  <strong>{state.result.authFile}</strong>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
