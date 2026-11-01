export type DemoService = {
  name: string;
  description: string;
  region: string;
  latency: string;
  errorRate: string;
  state: "Operational" | "Degraded" | "Incident";
  initials: string;
  tone: string;
};

export type DemoIncident = {
  id: string;
  title: string;
  serviceName: string;
  region: string;
  severity: "critical" | "medium";
  status: "investigating" | "monitoring";
  startedAt: string;
  source: "demo_seed";
};

export const demoServices: DemoService[] = [
  { name: "api-gateway", description: "Edge request router", region: "eu-central-1", latency: "42 ms", errorRate: "0.03%", state: "Operational", initials: "AG", tone: "blue" },
  { name: "identity-core", description: "Authentication service", region: "eu-central-1", latency: "68 ms", errorRate: "0.08%", state: "Operational", initials: "ID", tone: "violet" },
  { name: "billing-engine", description: "Payments & invoicing", region: "eu-west-1", latency: "184 ms", errorRate: "1.42%", state: "Degraded", initials: "BE", tone: "amber" },
  { name: "event-stream", description: "Async event processing", region: "eu-central-1", latency: "31 ms", errorRate: "0.01%", state: "Operational", initials: "ES", tone: "teal" },
  { name: "search-indexer", description: "Search infrastructure", region: "us-east-1", latency: "—", errorRate: "—", state: "Incident", initials: "SI", tone: "rose" },
];

export const demoIncidents: DemoIncident[] = [
  {
    id: "demo-search-indexing",
    title: "Search indexing stalled",
    serviceName: "search-indexer",
    region: "us-east-1",
    severity: "critical",
    status: "investigating",
    startedAt: new Date(Date.now() - 14 * 60 * 1000).toISOString(),
    source: "demo_seed",
  },
  {
    id: "demo-payment-latency",
    title: "Elevated payment latency",
    serviceName: "billing-engine",
    region: "eu-west-1",
    severity: "medium",
    status: "monitoring",
    startedAt: new Date(Date.now() - 38 * 60 * 1000).toISOString(),
    source: "demo_seed",
  },
];

export const chartPoints =
  "0,118 26,110 52,114 78,92 104,99 130,77 156,84 182,72 208,88 234,62 260,70 286,55 312,68 338,45 364,51 390,37 416,48 442,31 468,39 494,20 520,34 546,23 572,29 598,12";
export const chartArea = `0,136 ${chartPoints} 598,136`;
