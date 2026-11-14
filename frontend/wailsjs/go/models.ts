export namespace models {
	
	export class ActionDefinition {
	    name: string;
	    title: string;
	    description?: string;
	    argumentsSchema?: number[];
	    uiSchema?: number[];
	    resultType: string;
	    timeoutSeconds?: number;
	    mutating?: boolean;
	    requiresConfirmation?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ActionDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.argumentsSchema = source["argumentsSchema"];
	        this.uiSchema = source["uiSchema"];
	        this.resultType = source["resultType"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.mutating = source["mutating"];
	        this.requiresConfirmation = source["requiresConfirmation"];
	    }
	}
	export class ActionResult {
	    type: string;
	    data?: number[];
	    fileName?: string;
	    mediaType?: string;
	    taskId?: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.data = source["data"];
	        this.fileName = source["fileName"];
	        this.mediaType = source["mediaType"];
	        this.taskId = source["taskId"];
	    }
	}
	export class CertKeyPair {
	    cert: string;
	    pk: string;
	
	    static createFrom(source: any = {}) {
	        return new CertKeyPair(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cert = source["cert"];
	        this.pk = source["pk"];
	    }
	}
	export class AgentTLS {
	    ca: CertKeyPair;
	    server: CertKeyPair;
	    client: CertKeyPair;
	
	    static createFrom(source: any = {}) {
	        return new AgentTLS(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ca = this.convertValues(source["ca"], CertKeyPair);
	        this.server = this.convertValues(source["server"], CertKeyPair);
	        this.client = this.convertValues(source["client"], CertKeyPair);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Agent {
	    addr: string;
	    tls?: AgentTLS;
	    monitoringDisabled?: boolean;
	    profilingEnabled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Agent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.addr = source["addr"];
	        this.tls = this.convertValues(source["tls"], AgentTLS);
	        this.monitoringDisabled = source["monitoringDisabled"];
	        this.profilingEnabled = source["profilingEnabled"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AgentDeploymentSettings {
	    address: string;
	    mtls: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentDeploymentSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.mtls = source["mtls"];
	    }
	}
	export class Registry {
	    sourceId?: string;
	    username?: string;
	    password?: string;
	    plainHttp?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Registry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceId = source["sourceId"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.plainHttp = source["plainHttp"];
	    }
	}
	export class AgentDeployment {
	    artifact?: string;
	    binaryPath?: string;
	    registry: Registry;
	    settings?: AgentDeploymentSettings;
	
	    static createFrom(source: any = {}) {
	        return new AgentDeployment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.artifact = source["artifact"];
	        this.binaryPath = source["binaryPath"];
	        this.registry = this.convertValues(source["registry"], Registry);
	        this.settings = this.convertValues(source["settings"], AgentDeploymentSettings);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class AgentReleaseAsset {
	    name: string;
	    version: string;
	    arch: string;
	    userspace: boolean;
	    url: string;
	    size: number;
	    publishedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentReleaseAsset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.arch = source["arch"];
	        this.userspace = source["userspace"];
	        this.url = source["url"];
	        this.size = source["size"];
	        this.publishedAt = source["publishedAt"];
	    }
	}
	export class AgentSource {
	    id: number[];
	    name: string;
	    url?: string;
	    path?: string;
	    cacheLocally?: boolean;
	    image?: string;
	    userspace?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.url = source["url"];
	        this.path = source["path"];
	        this.cacheLocally = source["cacheLocally"];
	        this.image = source["image"];
	        this.userspace = source["userspace"];
	    }
	}
	
	
	export class Condition {
	    type: string;
	    status: string;
	    reason: string;
	    message: string;
	    observedGeneration?: number;
	    // Go type: time
	    lastTransitionTime: any;
	
	    static createFrom(source: any = {}) {
	        return new Condition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	        this.message = source["message"];
	        this.observedGeneration = source["observedGeneration"];
	        this.lastTransitionTime = this.convertValues(source["lastTransitionTime"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DeployStatus {
	    step: string;
	    done: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeployStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.step = source["step"];
	        this.done = source["done"];
	        this.error = source["error"];
	    }
	}
	export class HostInfo {
	    backend: string;
	    version: string;
	    docker: boolean;
	    inDocker: boolean;
	    kernelModule: boolean;
	    interfaceKinds: string[];
	    arch: string;
	
	    static createFrom(source: any = {}) {
	        return new HostInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.version = source["version"];
	        this.docker = source["docker"];
	        this.inDocker = source["inDocker"];
	        this.kernelModule = source["kernelModule"];
	        this.interfaceKinds = source["interfaceKinds"];
	        this.arch = source["arch"];
	    }
	}
	export class ImportPreviewHooks {
	    preUp?: string[];
	    postUp?: string[];
	    preDown?: string[];
	    postDown?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportPreviewHooks(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preUp = source["preUp"];
	        this.postUp = source["postUp"];
	        this.preDown = source["preDown"];
	        this.postDown = source["postDown"];
	    }
	}
	export class ImportPreviewPeer {
	    name?: string;
	    publicKey: string;
	    allowedIPs: string[];
	    endpoint?: string;
	    keepalive: number;
	    presharedKey: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportPreviewPeer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.publicKey = source["publicKey"];
	        this.allowedIPs = source["allowedIPs"];
	        this.endpoint = source["endpoint"];
	        this.keepalive = source["keepalive"];
	        this.presharedKey = source["presharedKey"];
	    }
	}
	export class ImportPreviewUser {
	    name: string;
	    exists: boolean;
	    peers: ImportPreviewPeer[];
	
	    static createFrom(source: any = {}) {
	        return new ImportPreviewUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.exists = source["exists"];
	        this.peers = this.convertValues(source["peers"], ImportPreviewPeer);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportPreview {
	    interface: string;
	    source: string;
	    live: boolean;
	    liveKind?: string;
	    address: string;
	    listenPort: number;
	    amnezia: boolean;
	    mtu?: number;
	    dns?: string[];
	    table?: number;
	    users: ImportPreviewUser[];
	    embeddedPeers: ImportPreviewPeer[];
	    hooks: ImportPreviewHooks;
	    generatedHooks: ImportPreviewHooks;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.interface = source["interface"];
	        this.source = source["source"];
	        this.live = source["live"];
	        this.liveKind = source["liveKind"];
	        this.address = source["address"];
	        this.listenPort = source["listenPort"];
	        this.amnezia = source["amnezia"];
	        this.mtu = source["mtu"];
	        this.dns = source["dns"];
	        this.table = source["table"];
	        this.users = this.convertValues(source["users"], ImportPreviewUser);
	        this.embeddedPeers = this.convertValues(source["embeddedPeers"], ImportPreviewPeer);
	        this.hooks = this.convertValues(source["hooks"], ImportPreviewHooks);
	        this.generatedHooks = this.convertValues(source["generatedHooks"], ImportPreviewHooks);
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class InterfacePeer {
	    key: number[];
	    psk?: number[];
	    ips: string[];
	    endpoint?: string;
	    disabled?: boolean;
	    keepalive: number;
	
	    static createFrom(source: any = {}) {
	        return new InterfacePeer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.psk = source["psk"];
	        this.ips = source["ips"];
	        this.endpoint = source["endpoint"];
	        this.disabled = source["disabled"];
	        this.keepalive = source["keepalive"];
	    }
	}
	export class Interface {
	    id: number[];
	    iface: string;
	    pk: number[];
	    listen: number;
	    addr: string;
	    mtu?: number;
	    dns?: string[];
	    disabled?: boolean;
	    table?: number;
	    fwMark?: number;
	    preUp?: string[];
	    postUp?: string[];
	    preDown?: string[];
	    postDown?: string[];
	    peers?: InterfacePeer[];
	    jc?: number;
	    jmin?: number;
	    jmax?: number;
	    s1?: number;
	    s2?: number;
	    s3?: number;
	    s4?: number;
	    h1?: string;
	    h2?: string;
	    h3?: string;
	    h4?: string;
	    i1?: string;
	    i2?: string;
	    i3?: string;
	    i4?: string;
	    i5?: string;
	    provider?: string;
	    resourceName?: string;
	    inSync: boolean;
	    lastSyncError?: string;
	    // Go type: time
	    lastSyncedAt: any;
	    tunnel?: number[];
	
	    static createFrom(source: any = {}) {
	        return new Interface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.iface = source["iface"];
	        this.pk = source["pk"];
	        this.listen = source["listen"];
	        this.addr = source["addr"];
	        this.mtu = source["mtu"];
	        this.dns = source["dns"];
	        this.disabled = source["disabled"];
	        this.table = source["table"];
	        this.fwMark = source["fwMark"];
	        this.preUp = source["preUp"];
	        this.postUp = source["postUp"];
	        this.preDown = source["preDown"];
	        this.postDown = source["postDown"];
	        this.peers = this.convertValues(source["peers"], InterfacePeer);
	        this.jc = source["jc"];
	        this.jmin = source["jmin"];
	        this.jmax = source["jmax"];
	        this.s1 = source["s1"];
	        this.s2 = source["s2"];
	        this.s3 = source["s3"];
	        this.s4 = source["s4"];
	        this.h1 = source["h1"];
	        this.h2 = source["h2"];
	        this.h3 = source["h3"];
	        this.h4 = source["h4"];
	        this.i1 = source["i1"];
	        this.i2 = source["i2"];
	        this.i3 = source["i3"];
	        this.i4 = source["i4"];
	        this.i5 = source["i5"];
	        this.provider = source["provider"];
	        this.resourceName = source["resourceName"];
	        this.inSync = source["inSync"];
	        this.lastSyncError = source["lastSyncError"];
	        this.lastSyncedAt = this.convertValues(source["lastSyncedAt"], null);
	        this.tunnel = source["tunnel"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class InterfaceConfig {
	    iface: string;
	    pk: number[];
	    listen: number;
	    addr: string;
	    mtu?: number;
	    dns?: string[];
	    disabled?: boolean;
	    table?: number;
	    fwMark?: number;
	    preUp?: string[];
	    postUp?: string[];
	    preDown?: string[];
	    postDown?: string[];
	    peers?: InterfacePeer[];
	    jc?: number;
	    jmin?: number;
	    jmax?: number;
	    s1?: number;
	    s2?: number;
	    s3?: number;
	    s4?: number;
	    h1?: string;
	    h2?: string;
	    h3?: string;
	    h4?: string;
	    i1?: string;
	    i2?: string;
	    i3?: string;
	    i4?: string;
	    i5?: string;
	
	    static createFrom(source: any = {}) {
	        return new InterfaceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.iface = source["iface"];
	        this.pk = source["pk"];
	        this.listen = source["listen"];
	        this.addr = source["addr"];
	        this.mtu = source["mtu"];
	        this.dns = source["dns"];
	        this.disabled = source["disabled"];
	        this.table = source["table"];
	        this.fwMark = source["fwMark"];
	        this.preUp = source["preUp"];
	        this.postUp = source["postUp"];
	        this.preDown = source["preDown"];
	        this.postDown = source["postDown"];
	        this.peers = this.convertValues(source["peers"], InterfacePeer);
	        this.jc = source["jc"];
	        this.jmin = source["jmin"];
	        this.jmax = source["jmax"];
	        this.s1 = source["s1"];
	        this.s2 = source["s2"];
	        this.s3 = source["s3"];
	        this.s4 = source["s4"];
	        this.h1 = source["h1"];
	        this.h2 = source["h2"];
	        this.h3 = source["h3"];
	        this.h4 = source["h4"];
	        this.i1 = source["i1"];
	        this.i2 = source["i2"];
	        this.i3 = source["i3"];
	        this.i4 = source["i4"];
	        this.i5 = source["i5"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PeerHistoryPoint {
	    // Go type: time
	    timestamp: any;
	    rx: number;
	    tx: number;
	    // Go type: time
	    lastHandshake: any;
	
	    static createFrom(source: any = {}) {
	        return new PeerHistoryPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.rx = source["rx"];
	        this.tx = source["tx"];
	        this.lastHandshake = this.convertValues(source["lastHandshake"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PeerHistory {
	    publicKey: string;
	    points: PeerHistoryPoint[];
	
	    static createFrom(source: any = {}) {
	        return new PeerHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.publicKey = source["publicKey"];
	        this.points = this.convertValues(source["points"], PeerHistoryPoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class InterfaceHistory {
	    interface: string;
	    peers: PeerHistory[];
	
	    static createFrom(source: any = {}) {
	        return new InterfaceHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.interface = source["interface"];
	        this.peers = this.convertValues(source["peers"], PeerHistory);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PeerSnapshot {
	    publicKey: string;
	    rx: number;
	    tx: number;
	    // Go type: time
	    lastHandshake: any;
	    probeAddress?: string;
	    latencyP95Ms?: number;
	
	    static createFrom(source: any = {}) {
	        return new PeerSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.publicKey = source["publicKey"];
	        this.rx = source["rx"];
	        this.tx = source["tx"];
	        this.lastHandshake = this.convertValues(source["lastHandshake"], null);
	        this.probeAddress = source["probeAddress"];
	        this.latencyP95Ms = source["latencyP95Ms"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class InterfaceSnapshot {
	    interface: string;
	    peers: PeerSnapshot[];
	
	    static createFrom(source: any = {}) {
	        return new InterfaceSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.interface = source["interface"];
	        this.peers = this.convertValues(source["peers"], PeerSnapshot);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LinkStatus {
	    observedGeneration: number;
	    phase: string;
	    conditions?: Condition[];
	
	    static createFrom(source: any = {}) {
	        return new LinkStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.observedGeneration = source["observedGeneration"];
	        this.phase = source["phase"];
	        this.conditions = this.convertValues(source["conditions"], Condition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LinkEndpoint {
	    serverId: string;
	    resourceId: string;
	    apiVersion: string;
	    kind: string;
	    name: string;
	    port: string;
	    address?: string;
	
	    static createFrom(source: any = {}) {
	        return new LinkEndpoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.resourceId = source["resourceId"];
	        this.apiVersion = source["apiVersion"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.port = source["port"];
	        this.address = source["address"];
	    }
	}
	export class Link {
	    id: string;
	    name?: string;
	    kind: string;
	    generation: number;
	    source: LinkEndpoint;
	    target: LinkEndpoint;
	    spec?: number[];
	    status: LinkStatus;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.generation = source["generation"];
	        this.source = this.convertValues(source["source"], LinkEndpoint);
	        this.target = this.convertValues(source["target"], LinkEndpoint);
	        this.spec = source["spec"];
	        this.status = this.convertValues(source["status"], LinkStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class PortDefinition {
	    name: string;
	    title?: string;
	    direction?: string;
	    linkKinds?: string[];
	    remotePorts?: string[];
	
	    static createFrom(source: any = {}) {
	        return new PortDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.title = source["title"];
	        this.direction = source["direction"];
	        this.linkKinds = source["linkKinds"];
	        this.remotePorts = source["remotePorts"];
	    }
	}
	export class ResourceDefinition {
	    apiVersion: string;
	    kind: string;
	    specSchema: number[];
	    uiSchema?: number[];
	    schemaHash: string;
	    ports?: PortDefinition[];
	    actions?: ActionDefinition[];
	
	    static createFrom(source: any = {}) {
	        return new ResourceDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.kind = source["kind"];
	        this.specSchema = source["specSchema"];
	        this.uiSchema = source["uiSchema"];
	        this.schemaHash = source["schemaHash"];
	        this.ports = this.convertValues(source["ports"], PortDefinition);
	        this.actions = this.convertValues(source["actions"], ActionDefinition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProviderDefinition {
	    name: string;
	    version: string;
	    resources: ResourceDefinition[];
	
	    static createFrom(source: any = {}) {
	        return new ProviderDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.resources = this.convertValues(source["resources"], ResourceDefinition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Manifest {
	    protocolVersion: string;
	    providers: ProviderDefinition[];
	    providerErrors?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Manifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocolVersion = source["protocolVersion"];
	        this.providers = this.convertValues(source["providers"], ProviderDefinition);
	        this.providerErrors = source["providerErrors"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NetworkConnection {
	    id: number[];
	    sourceId: number[];
	    targetId: number[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sourceId = source["sourceId"];
	        this.targetId = source["targetId"];
	    }
	}
	export class NetworkInterface {
	    id: number[];
	    serverId: number[];
	    name: string;
	    address?: string;
	    legacy?: boolean;
	    serviceId?: number[];
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new NetworkInterface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.name = source["name"];
	        this.address = source["address"];
	        this.legacy = source["legacy"];
	        this.serviceId = source["serviceId"];
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class NodePosition {
	    id: number[];
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new NodePosition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class Peer {
	    name: string;
	    pk: number[];
	    interface: number[];
	    provider?: string;
	    resourceName?: string;
	    disabled?: boolean;
	    dns?: string[];
	
	    static createFrom(source: any = {}) {
	        return new Peer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.pk = source["pk"];
	        this.interface = source["interface"];
	        this.provider = source["provider"];
	        this.resourceName = source["resourceName"];
	        this.disabled = source["disabled"];
	        this.dns = source["dns"];
	    }
	}
	
	
	
	export class PluginDescriptor {
	    id: string;
	    name: string;
	    version: string;
	    description?: string;
	    configSchema: Record<string, any>;
	    userSchema: Record<string, any>;
	    uiSchema?: Record<string, any>;
	    available: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PluginDescriptor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.description = source["description"];
	        this.configSchema = source["configSchema"];
	        this.userSchema = source["userSchema"];
	        this.uiSchema = source["uiSchema"];
	        this.available = source["available"];
	        this.error = source["error"];
	    }
	}
	export class PluginUser {
	    id: string;
	    data: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new PluginUser(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.data = source["data"];
	    }
	}
	
	
	export class ProviderDeployment {
	    name: string;
	    artifact?: string;
	    binaryPath?: string;
	    options?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new ProviderDeployment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.artifact = source["artifact"];
	        this.binaryPath = source["binaryPath"];
	        this.options = source["options"];
	    }
	}
	export class ProviderRepository {
	    name: string;
	    repository: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderRepository(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.repository = source["repository"];
	    }
	}
	
	export class RegistrySource {
	    id: number[];
	    url: string;
	    type: string;
	    username?: string;
	    password?: string;
	    hasPassword?: boolean;
	    plainHttp?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RegistrySource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.type = source["type"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.hasPassword = source["hasPassword"];
	        this.plainHttp = source["plainHttp"];
	    }
	}
	export class ResourceStatus {
	    observedGeneration: number;
	    phase: string;
	    conditions?: Condition[];
	    provider?: number[];
	
	    static createFrom(source: any = {}) {
	        return new ResourceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.observedGeneration = source["observedGeneration"];
	        this.phase = source["phase"];
	        this.conditions = this.convertValues(source["conditions"], Condition);
	        this.provider = source["provider"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Resource {
	    id: string;
	    serverId: string;
	    apiVersion: string;
	    kind: string;
	    name: string;
	    generation: number;
	    spec: number[];
	    status: ResourceStatus;
	
	    static createFrom(source: any = {}) {
	        return new Resource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.apiVersion = source["apiVersion"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.generation = source["generation"];
	        this.spec = source["spec"];
	        this.status = this.convertValues(source["status"], ResourceStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class SSHConfig {
	    host: string;
	    port?: number;
	    user?: string;
	    key?: string;
	    keyData?: string;
	    password?: string;
	
	    static createFrom(source: any = {}) {
	        return new SSHConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.key = source["key"];
	        this.keyData = source["keyData"];
	        this.password = source["password"];
	    }
	}
	export class ServerInfo {
	    description?: string;
	    location?: string;
	    tags?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ServerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.description = source["description"];
	        this.location = source["location"];
	        this.tags = source["tags"];
	    }
	}
	export class Server {
	    id: number[];
	    name: string;
	    info: ServerInfo;
	    ssh: SSHConfig;
	    agent: Agent;
	    interfaces?: number[][];
	    unmanaged?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.info = this.convertValues(source["info"], ServerInfo);
	        this.ssh = this.convertValues(source["ssh"], SSHConfig);
	        this.agent = this.convertValues(source["agent"], Agent);
	        this.interfaces = source["interfaces"];
	        this.unmanaged = source["unmanaged"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SystemHistoryPoint {
	    // Go type: time
	    timestamp: any;
	    cpuPercent: number;
	    memUsedBytes: number;
	    memTotalBytes: number;
	    netRxBytes: number;
	    netTxBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new SystemHistoryPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.cpuPercent = source["cpuPercent"];
	        this.memUsedBytes = source["memUsedBytes"];
	        this.memTotalBytes = source["memTotalBytes"];
	        this.netRxBytes = source["netRxBytes"];
	        this.netTxBytes = source["netTxBytes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SystemHistory {
	    points: SystemHistoryPoint[];
	
	    static createFrom(source: any = {}) {
	        return new SystemHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.points = this.convertValues(source["points"], SystemHistoryPoint);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SystemSnapshot {
	    // Go type: time
	    timestamp: any;
	    cpuPercent: number;
	    memUsedBytes: number;
	    memTotalBytes: number;
	    load1: number;
	    load5: number;
	    load15: number;
	    netRxBytes: number;
	    netTxBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new SystemSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.cpuPercent = source["cpuPercent"];
	        this.memUsedBytes = source["memUsedBytes"];
	        this.memTotalBytes = source["memTotalBytes"];
	        this.load1 = source["load1"];
	        this.load5 = source["load5"];
	        this.load15 = source["load15"];
	        this.netRxBytes = source["netRxBytes"];
	        this.netTxBytes = source["netTxBytes"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VPNService {
	    id: number[];
	    serverId: number[];
	    interfaceId?: number[];
	    pluginId: string;
	    name: string;
	    config: Record<string, any>;
	    status: string;
	    statusMessage?: string;
	    // Go type: time
	    lastAppliedAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new VPNService(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.serverId = source["serverId"];
	        this.interfaceId = source["interfaceId"];
	        this.pluginId = source["pluginId"];
	        this.name = source["name"];
	        this.config = source["config"];
	        this.status = source["status"];
	        this.statusMessage = source["statusMessage"];
	        this.lastAppliedAt = this.convertValues(source["lastAppliedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TopologyServer {
	    id: number[];
	    name: string;
	    address?: string;
	    description?: string;
	    location?: string;
	    locationError?: string;
	    legacy?: boolean;
	    latitude?: number;
	    longitude?: number;
	    coordinatesManual?: boolean;
	    x: number;
	    y: number;
	
	    static createFrom(source: any = {}) {
	        return new TopologyServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.address = source["address"];
	        this.description = source["description"];
	        this.location = source["location"];
	        this.locationError = source["locationError"];
	        this.legacy = source["legacy"];
	        this.latitude = source["latitude"];
	        this.longitude = source["longitude"];
	        this.coordinatesManual = source["coordinatesManual"];
	        this.x = source["x"];
	        this.y = source["y"];
	    }
	}
	export class Topology {
	    servers: TopologyServer[];
	    interfaces: NetworkInterface[];
	    services: VPNService[];
	    connections: NetworkConnection[];
	
	    static createFrom(source: any = {}) {
	        return new Topology(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.servers = this.convertValues(source["servers"], TopologyServer);
	        this.interfaces = this.convertValues(source["interfaces"], NetworkInterface);
	        this.services = this.convertValues(source["services"], VPNService);
	        this.connections = this.convertValues(source["connections"], NetworkConnection);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class TunnelMember {
	    serverId: number[];
	    serverName: string;
	    ifaceId: number[];
	    interface: string;
	    role: string;
	
	    static createFrom(source: any = {}) {
	        return new TunnelMember(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.serverName = source["serverName"];
	        this.ifaceId = source["ifaceId"];
	        this.interface = source["interface"];
	        this.role = source["role"];
	    }
	}
	export class Tunnel {
	    id: number[];
	    members: TunnelMember[];
	
	    static createFrom(source: any = {}) {
	        return new Tunnel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.members = this.convertValues(source["members"], TunnelMember);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class TunnelMetric {
	    connectionId: string;
	    latencyP95Ms?: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new TunnelMetric(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.latencyP95Ms = source["latencyP95Ms"];
	        this.error = source["error"];
	    }
	}
	export class TunnelStep {
	    serverId: number[];
	    ifaceId: number[];
	
	    static createFrom(source: any = {}) {
	        return new TunnelStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.ifaceId = source["ifaceId"];
	    }
	}
	export class UnmanagedInterface {
	    name: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new UnmanagedInterface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	    }
	}
	export class User {
	    id: number[];
	    name: string;
	    description?: string;
	    disabled?: boolean;
	    peers?: Peer[];
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.disabled = source["disabled"];
	        this.peers = this.convertValues(source["peers"], Peer);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace service {
	
	export class AddPeerInput {
	    Name: string;
	    InterfaceID: number[];
	    Spec: Record<string, any>;
	    AllowedIPs: string[];
	    Endpoint: string;
	    DNS: string[];
	    PrivateKey: string;
	    PresharedKey: string;
	    WithPresharedKey: boolean;
	    KeepaliveInterval: number;
	
	    static createFrom(source: any = {}) {
	        return new AddPeerInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.InterfaceID = source["InterfaceID"];
	        this.Spec = source["Spec"];
	        this.AllowedIPs = source["AllowedIPs"];
	        this.Endpoint = source["Endpoint"];
	        this.DNS = source["DNS"];
	        this.PrivateKey = source["PrivateKey"];
	        this.PresharedKey = source["PresharedKey"];
	        this.WithPresharedKey = source["WithPresharedKey"];
	        this.KeepaliveInterval = source["KeepaliveInterval"];
	    }
	}
	export class NetworkInterfaceInput {
	    name: string;
	    address: string;
	
	    static createFrom(source: any = {}) {
	        return new NetworkInterfaceInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.address = source["address"];
	    }
	}
	export class ProfileDump {
	    filename: string;
	    data: number[];
	
	    static createFrom(source: any = {}) {
	        return new ProfileDump(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.filename = source["filename"];
	        this.data = source["data"];
	    }
	}
	export class ReconcileReport {
	    agentOnly: models.InterfaceConfig[];
	    dbOnly: models.Interface[];
	
	    static createFrom(source: any = {}) {
	        return new ReconcileReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentOnly = this.convertValues(source["agentOnly"], models.InterfaceConfig);
	        this.dbOnly = this.convertValues(source["dbOnly"], models.Interface);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ServerInput {
	    Name: string;
	    Info: models.ServerInfo;
	    SSH: models.SSHConfig;
	    Agent: models.Agent;
	
	    static createFrom(source: any = {}) {
	        return new ServerInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Info = this.convertValues(source["Info"], models.ServerInfo);
	        this.SSH = this.convertValues(source["SSH"], models.SSHConfig);
	        this.Agent = this.convertValues(source["Agent"], models.Agent);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TopologyServerInput {
	    name: string;
	    address: string;
	    description: string;
	    location: string;
	    latitude?: number;
	    longitude?: number;
	    interfaces: NetworkInterfaceInput[];
	
	    static createFrom(source: any = {}) {
	        return new TopologyServerInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.address = source["address"];
	        this.description = source["description"];
	        this.location = source["location"];
	        this.latitude = source["latitude"];
	        this.longitude = source["longitude"];
	        this.interfaces = this.convertValues(source["interfaces"], NetworkInterfaceInput);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UserInput {
	    Name: string;
	    Description: string;
	    Disabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new UserInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Description = source["Description"];
	        this.Disabled = source["Disabled"];
	    }
	}
	export class VPNServiceInput {
	    serverId: string;
	    interfaceId: string;
	    pluginId: string;
	    name: string;
	    config: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new VPNServiceInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverId = source["serverId"];
	        this.interfaceId = source["interfaceId"];
	        this.pluginId = source["pluginId"];
	        this.name = source["name"];
	        this.config = source["config"];
	    }
	}

}

