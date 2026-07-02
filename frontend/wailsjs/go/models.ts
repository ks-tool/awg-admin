export namespace models {
	
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
	
	    static createFrom(source: any = {}) {
	        return new Agent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.addr = source["addr"];
	        this.tls = this.convertValues(source["tls"], AgentTLS);
	        this.monitoringDisabled = source["monitoringDisabled"];
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
	export class AgentSource {
	    id: number[];
	    name: string;
	    url?: string;
	    path?: string;
	    cacheLocally?: boolean;
	    image?: string;
	
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
	    inSync: boolean;
	    lastSyncError?: string;
	    // Go type: time
	    lastSyncedAt?: any;
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
	
	    static createFrom(source: any = {}) {
	        return new PeerSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.publicKey = source["publicKey"];
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
	export class MetricsSnapshot {
	    system: SystemSnapshot;
	    interfaces: InterfaceSnapshot[];
	
	    static createFrom(source: any = {}) {
	        return new MetricsSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.system = this.convertValues(source["system"], SystemSnapshot);
	        this.interfaces = this.convertValues(source["interfaces"], InterfaceSnapshot);
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
	export class Peer {
	    name: string;
	    pk: number[];
	    interface: number[];
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
	        this.disabled = source["disabled"];
	        this.dns = source["dns"];
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
	    interfaces: InterfaceHistory[];
	
	    static createFrom(source: any = {}) {
	        return new SystemHistory(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.points = this.convertValues(source["points"], SystemHistoryPoint);
	        this.interfaces = this.convertValues(source["interfaces"], InterfaceHistory);
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
	        this.AllowedIPs = source["AllowedIPs"];
	        this.Endpoint = source["Endpoint"];
	        this.DNS = source["DNS"];
	        this.PrivateKey = source["PrivateKey"];
	        this.PresharedKey = source["PresharedKey"];
	        this.WithPresharedKey = source["WithPresharedKey"];
	        this.KeepaliveInterval = source["KeepaliveInterval"];
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

}

