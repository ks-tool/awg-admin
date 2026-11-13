export interface ProviderManifest {
    protocolVersion: string;
    providers: ProviderDefinition[];
    providerErrors?: Record<string, string>;
}

export interface ProviderDefinition {
    name: string;
    version: string;
    resources: ResourceDefinition[];
}

export interface ResourceDefinition {
    apiVersion: string;
    kind: string;
    specSchema: Record<string, unknown>;
    uiSchema?: Record<string, unknown>;
    schemaHash: string;
    ports?: PortDefinition[];
    actions?: ActionDefinition[];
}

export interface PortDefinition {
    name: string;
    title?: string;
    direction?: string;
    linkKinds?: string[];
    remotePorts?: string[];
}

export interface ActionDefinition {
    name: string;
    title: string;
    description?: string;
    argumentsSchema?: Record<string, unknown>;
    uiSchema?: Record<string, unknown>;
    resultType: 'json' | 'file' | 'async';
    timeoutSeconds?: number;
    mutating?: boolean;
    requiresConfirmation?: boolean;
}

export interface ProviderResource {
    id: string;
    serverId: string;
    apiVersion: string;
    kind: string;
    name: string;
    generation: number;
    spec: Record<string, unknown>;
    status: ResourceStatus;
}

export interface ResourceStatus {
    observedGeneration: number;
    phase: 'Pending' | 'Ready' | 'Degraded' | 'Deleting';
    conditions?: ResourceCondition[];
    provider?: Record<string, unknown>;
}

export interface ResourceCondition {
    type: string;
    status: 'True' | 'False' | 'Unknown';
    reason: string;
    message: string;
    observedGeneration?: number;
    lastTransitionTime?: string;
}

export interface ActionResult {
    type: 'json' | 'file' | 'async';
    data?: unknown;
    fileName?: string;
    mediaType?: string;
    taskId?: string;
}

export interface ProviderLink {
    id: string;
    name?: string;
    kind: string;
    generation: number;
    source: LinkEndpoint;
    target: LinkEndpoint;
    spec?: Record<string, unknown>;
    status: LinkStatus;
}

export interface LinkEndpoint {
    serverId: string;
    resourceId: string;
    apiVersion: string;
    kind: string;
    name: string;
    port: string;
    address?: string;
}

export interface LinkStatus {
    observedGeneration: number;
    phase: 'Pending' | 'Preparing' | 'Ready' | 'Degraded';
    conditions?: ResourceCondition[];
}
