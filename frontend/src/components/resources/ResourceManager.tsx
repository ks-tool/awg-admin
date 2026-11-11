/*
  Copyright © 2026 Alexey Shulutkov <github@shulutkov.ru>

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  	http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Edit2, FileInput, Network, Play, Plus, Power, RefreshCw, Trash2} from 'lucide-react';
import {toast} from 'sonner';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {FormField} from '@/components/common/FormField';
import {Modal} from '@/components/common/Modal';
import {buttons, inputs} from '@/components/common/controls';
import {ImportInterfaceModal} from '@/components/server/ImportInterfaceModal';
import {SchemaForm} from '@/components/topology/SchemaForm';
import {listUnmanagedInterfaces} from '@/services/servers';
import {
    deleteProviderResource,
    getProviderManifest,
    getProviderSchema,
    listProviderResources,
    putProviderResource,
    runProviderResourceAction,
} from '@/services/resources';
import {cn} from '@/lib/utils';
import type {ActionDefinition, ActionResult, ProviderDefinition, ProviderManifest, ProviderResource, ResourceDefinition, UnmanagedInterface} from '@/types';

interface ListedResource {
    provider: string;
    definition: ResourceDefinition;
    resource: ProviderResource;
}

interface EditorState {
    provider: string;
    kind: string;
    resource?: ProviderResource;
}

interface ActionState {
    item: ListedResource;
    action: ActionDefinition;
}

export function ResourceManager({serverId, onChanged}: {serverId: string; onChanged?: () => void}) {
    const {t} = useTranslation();
    const [manifest, setManifest] = useState<ProviderManifest | null>(null);
    const [schemas, setSchemas] = useState<Record<string, ProviderDefinition>>({});
    const [resources, setResources] = useState<ListedResource[]>([]);
    const [unmanaged, setUnmanaged] = useState<UnmanagedInterface[]>([]);
    const [loading, setLoading] = useState(true);
    const [editor, setEditor] = useState<EditorState | null>(null);
    const [deleting, setDeleting] = useState<ListedResource | null>(null);
    const [action, setAction] = useState<ActionState | null>(null);
    const [importTarget, setImportTarget] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);

    const load = useCallback(async () => {
        const [nextManifest, nextUnmanaged] = await Promise.all([
            getProviderManifest(serverId),
            listUnmanagedInterfaces(serverId),
        ]);
        const definitions = await Promise.all(nextManifest.providers.map(provider => getProviderSchema(serverId, provider.name)));
        const nextSchemas = Object.fromEntries(definitions.map(definition => [definition.name, definition]));
        const lists = await Promise.all(nextManifest.providers.map(async provider => {
            const items = await listProviderResources(serverId, provider.name);
            return items.flatMap(resource => {
                if (resource.kind !== 'Interface') return [];
                const definition = nextSchemas[provider.name]?.resources.find(item => item.kind === resource.kind);
                return definition ? [{provider: provider.name, definition, resource}] : [];
            });
        }));
        setManifest(nextManifest);
        setSchemas(nextSchemas);
        setResources(lists.flat());
        setUnmanaged(nextUnmanaged ?? []);
    }, [serverId]);

    const refresh = useCallback(async () => {
        try {
            await load();
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('resources.loadError'));
        } finally {
            setLoading(false);
        }
    }, [load, t]);

    useEffect(() => {
        const timeout = window.setTimeout(() => void refresh(), 0);
        return () => window.clearTimeout(timeout);
    }, [refresh]);

    const save = useCallback(async (editorState: EditorState, name: string, spec: Record<string, unknown>) => {
        setBusy(true);
        try {
            await putProviderResource(serverId, editorState.provider, editorState.kind, name, spec);
            setEditor(null);
            await load();
            onChanged?.();
            toast.success(t('resources.saved'));
            window.setTimeout(() => void load().catch(() => undefined), 750);
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('resources.saveError'));
        } finally {
            setBusy(false);
        }
    }, [load, onChanged, serverId, t]);

    const toggleDisabled = useCallback(async (item: ListedResource) => {
        setBusy(true);
        try {
            const disabled = item.resource.spec.disabled === true;
            await putProviderResource(serverId, item.provider, item.resource.kind, item.resource.name, {...item.resource.spec, disabled: !disabled});
            await load();
            window.setTimeout(() => void load().catch(() => undefined), 750);
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('resources.saveError'));
        } finally {
            setBusy(false);
        }
    }, [load, serverId, t]);

    const remove = useCallback(async () => {
        if (!deleting) return;
        setBusy(true);
        try {
            await deleteProviderResource(serverId, deleting.provider, deleting.resource.kind, deleting.resource.name);
            setDeleting(null);
            await load();
            onChanged?.();
            toast.success(t('resources.deleted'));
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('resources.deleteError'));
        } finally {
            setBusy(false);
        }
    }, [deleting, load, onChanged, serverId, t]);

    const executeAction = useCallback(async (state: ActionState, args: Record<string, unknown>) => {
        setBusy(true);
        try {
            const result = await runProviderResourceAction(serverId, state.item.provider, state.item.resource.kind, state.item.resource.name, state.action.name, args);
            if (state.action.mutating) {
                await load();
                onChanged?.();
                window.setTimeout(() => void load().catch(() => undefined), 750);
            }
            return result;
        } finally {
            setBusy(false);
        }
    }, [load, onChanged, serverId]);

    const supportsDisabled = (definition: ResourceDefinition) => {
        const properties = definition.specSchema.properties as Record<string, {type?: string}> | undefined;
        return properties?.disabled?.type === 'boolean';
    };

    const interfaceProviders = manifest?.providers.filter(provider => provider.resources.some(resource => resource.kind === 'Interface')) ?? [];

    return (
        <div className="rounded-xl border border-border bg-card p-6 dark:border-white/5 dark:bg-white/3">
            <div className="mb-4 flex items-center justify-between">
                <h2 className="flex items-center gap-2 text-lg font-semibold"><Network size={20} />{t('servers.interface')} ({resources.length})</h2>
                <div className="flex gap-2">
                    <button type="button" onClick={() => void refresh()} disabled={loading} className={buttons.secondary} title={t('common.refresh')}><RefreshCw size={14} className={loading ? 'animate-spin' : ''} /></button>
                    <button
                        type="button"
                        onClick={() => {
                            const provider = interfaceProviders[0];
                            if (provider) setEditor({provider: provider.name, kind: 'Interface'});
                        }}
                        disabled={interfaceProviders.length === 0}
                        className={buttons.primary}
                        title={t('servers.addInterfaceTooltip')}
                    ><Plus size={14} /></button>
                </div>
            </div>
            {interfaceProviders.length > 0 && (
                <div className="mb-4 flex flex-wrap gap-2">
                    {interfaceProviders.map(provider => (
                        <div key={provider.name} className="rounded-lg border border-border bg-background px-3 py-2 text-xs dark:border-white/10 dark:bg-white/5">
                            <span className="font-semibold">{provider.name}</span>
                            <span className="ml-2 font-mono text-muted-foreground">{provider.version}</span>
                        </div>
                    ))}
                </div>
            )}
            {loading ? <div className="py-8 text-center text-sm text-muted-foreground">{t('common.loading')}</div> : resources.length === 0 && unmanaged.length === 0 ? (
                <div className="py-4 text-center text-muted-foreground">{t('servers.interfaces.noInterfaces')}</div>
            ) : (
                <div className="space-y-3">
                    {resources.map(item => {
                        const condition = item.resource.status.conditions?.[0];
                        const disabled = item.resource.spec.disabled === true;
                        return (
                            <div key={`${item.provider}:${item.resource.kind}:${item.resource.name}`} className="rounded-lg border border-input bg-background p-4 dark:border-white/10 dark:bg-white/5">
                                <div className="flex items-start justify-between gap-4">
                                    <div className="min-w-0">
                                        <div className="flex flex-wrap items-center gap-2 text-sm font-semibold">
                                            <span className={cn('h-2 w-2 rounded-full', item.resource.status.phase === 'Ready' ? 'bg-emerald-500' : item.resource.status.phase === 'Degraded' ? 'bg-red-500' : 'bg-amber-500')} />
                                            {item.resource.name}
                                            <span className="rounded bg-muted px-1.5 py-0.5 text-xs font-normal text-muted-foreground">{item.provider}/{item.resource.kind}</span>
                                        </div>
                                        <div className="mt-1 text-xs text-muted-foreground">{item.resource.status.phase} · {t('resources.generation')} {item.resource.status.observedGeneration}/{item.resource.generation}</div>
                                        {condition?.message && <div className={cn('mt-2 text-xs', condition.status === 'False' ? 'text-red-500' : 'text-muted-foreground')}>{condition.reason}: {condition.message}</div>}
                                    </div>
                                    <div className="flex shrink-0 gap-2">
                                        {item.definition.actions?.map(actionDefinition => <button key={actionDefinition.name} type="button" disabled={busy} onClick={() => setAction({item, action: actionDefinition})} title={actionDefinition.title} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"><Play size={15} /></button>)}
                                        {supportsDisabled(item.definition) && <button type="button" disabled={busy} onClick={() => void toggleDisabled(item)} title={disabled ? t('resources.activate') : t('resources.deactivate')} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"><Power size={16} /></button>}
                                        <button type="button" disabled={busy} onClick={() => setEditor({provider: item.provider, kind: item.resource.kind, resource: item.resource})} title={t('common.edit')} className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground"><Edit2 size={16} /></button>
                                        <button type="button" disabled={busy} onClick={() => setDeleting(item)} title={t('common.delete')} className="rounded p-1 text-muted-foreground hover:bg-red-100 hover:text-red-600 dark:hover:bg-red-500/10"><Trash2 size={16} /></button>
                                    </div>
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}
            {unmanaged.length > 0 && (
                <div className="mt-3 space-y-3">
                    {unmanaged.map(item => (
                        <div key={item.name} className="flex items-center justify-between rounded-lg border border-dashed border-amber-300/70 bg-background p-4 dark:border-amber-500/30 dark:bg-white/5">
                            <div className="flex items-center gap-2 text-sm font-semibold">
                                <Network size={16} />
                                {item.name}
                                <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-500/15 dark:text-amber-400">{t('servers.import.notManagedBadge')}</span>
                                <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs font-medium text-muted-foreground">{item.kind}</span>
                            </div>
                            <button type="button" onClick={() => setImportTarget(item.name)} className={cn(buttons.secondary, 'inline-flex items-center gap-1.5 px-3 py-1')} title={t('servers.import.rowHint')}>
                                <FileInput size={14} />{t('servers.import.button')}
                            </button>
                        </div>
                    ))}
                </div>
            )}
            {importTarget !== null && (
                <ImportInterfaceModal
                    serverId={serverId}
                    initialInterface={importTarget}
                    unmanaged={unmanaged}
                    onClose={() => setImportTarget(null)}
                    onImported={() => {
                        setImportTarget(null);
                        onChanged?.();
                        void refresh();
                    }}
                />
            )}
            {editor && schemas[editor.provider] && (
                <ResourceEditor
                    key={`${editor.provider}:${editor.kind}:${editor.resource?.id ?? 'new'}`}
                    state={editor}
                    manifest={manifest}
                    schemas={schemas}
                    busy={busy}
                    onStateChange={setEditor}
                    onClose={() => setEditor(null)}
                    onSave={(name, spec) => void save(editor, name, spec)}
                />
            )}
            {action && <ResourceAction state={action} busy={busy} onClose={() => setAction(null)} onExecute={executeAction} />}
            {deleting && <ConfirmModal title={t('resources.deleteTitle')} message={t('resources.deleteConfirm', {name: deleting.resource.name})} onCancel={() => setDeleting(null)} onConfirm={() => void remove()} loading={busy} />}
        </div>
    );
}

function ResourceAction({state, busy, onClose, onExecute}: {
    state: ActionState;
    busy: boolean;
    onClose: () => void;
    onExecute: (state: ActionState, args: Record<string, unknown>) => Promise<ActionResult>;
}) {
    const {t} = useTranslation();
    const [formData, setFormData] = useState<Record<string, unknown>>({});
    const [result, setResult] = useState<ActionResult | null>(null);
    const [error, setError] = useState<string | null>(null);
    const schema = state.action.argumentsSchema ?? {type: 'object', properties: {}};
    const execute = async (args: Record<string, unknown>) => {
        setError(null);
        try {
            setResult(await onExecute(state, args));
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : t('resources.actionError'));
        }
    };
    const resultText = result?.type === 'file' && typeof result.data === 'string'
        ? result.data
        : result ? JSON.stringify(result.data ?? {taskId: result.taskId}, null, 2) : '';
    const download = () => {
        if (!result || result.type !== 'file') return;
        const url = URL.createObjectURL(new Blob([resultText], {type: result.mediaType || 'text/plain'}));
        const anchor = document.createElement('a');
        anchor.href = url;
        anchor.download = result.fileName || 'result.txt';
        anchor.click();
        URL.revokeObjectURL(url);
    };

    return (
        <Modal title={state.action.title} onClose={onClose} loading={busy} size="lg">
            <div className="mb-3 text-sm text-muted-foreground">{state.item.resource.name} · {state.item.provider}/{state.item.resource.kind}</div>
            {state.action.description && <p className="mb-4 text-sm text-muted-foreground">{state.action.description}</p>}
            {state.action.requiresConfirmation && !result && <p className="mb-4 rounded-lg bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300">{t('resources.actionConfirm')}</p>}
            {error && <p className="mb-4 rounded-lg bg-red-500/10 p-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
            {result ? (
                <div>
                    <pre className="max-h-96 overflow-auto rounded-lg bg-muted p-4 text-xs whitespace-pre-wrap">{resultText}</pre>
                    <div className="mt-4 flex justify-end gap-2">
                        {result.type === 'file' && <button type="button" onClick={download} className={buttons.secondary}>{t('resources.download')}</button>}
                        <button type="button" onClick={onClose} className={buttons.primary}>{t('common.close')}</button>
                    </div>
                </div>
            ) : (
                <SchemaForm schema={schema} uiSchema={state.action.uiSchema} formData={formData} onChange={setFormData} onSubmit={execute} submitLabel={busy ? t('common.saving') : state.action.title} disabled={busy} />
            )}
        </Modal>
    );
}

function ResourceEditor({state, manifest, schemas, busy, onStateChange, onClose, onSave}: {
    state: EditorState;
    manifest: ProviderManifest | null;
    schemas: Record<string, ProviderDefinition>;
    busy: boolean;
    onStateChange: (state: EditorState) => void;
    onClose: () => void;
    onSave: (name: string, spec: Record<string, unknown>) => void;
}) {
    const {t} = useTranslation();
    const [name, setName] = useState(state.resource?.name ?? '');
    const [formData, setFormData] = useState<Record<string, unknown>>(state.resource?.spec ?? {});
    const definition = useMemo(() => schemas[state.provider]?.resources.find(item => item.kind === state.kind), [schemas, state.provider, state.kind]);
    if (!definition) return null;

    return (
        <Modal title={state.resource ? t('servers.interfaces.editTitle') : t('servers.interfaces.addTitle')} onClose={onClose} loading={busy} size="lg">
            <FormField label={t('resources.provider')}>
                <select value={state.provider} disabled={busy || !!state.resource} onChange={event => {
                    setFormData({});
                    onStateChange({provider: event.target.value, kind: 'Interface'});
                }} className={inputs.primary}>
                    {manifest?.providers.filter(provider => provider.resources.some(resource => resource.kind === 'Interface')).map(provider => <option key={provider.name} value={provider.name}>{provider.name} ({provider.version})</option>)}
                </select>
            </FormField>
            <FormField label={t('resources.name')}>
                <input required value={name} disabled={busy || !!state.resource} onChange={event => setName(event.target.value)} className={inputs.primary} />
            </FormField>
            <SchemaForm schema={definition.specSchema} uiSchema={definition.uiSchema} formData={formData} onChange={setFormData} onSubmit={spec => onSave(name.trim(), spec)} submitLabel={busy ? t('common.saving') : t('common.save')} disabled={busy || !name.trim()} />
        </Modal>
    );
}
