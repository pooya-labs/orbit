export type Language = "en" | "fa";

const messages = {
  en: {
    workspace: "Workspace", production: "PRODUCTION", observe: "OBSERVE", investigate: "INVESTIGATE", deliver: "DELIVER",
    overview: "Overview", serviceMap: "Service map", services: "Services", infrastructure: "Infrastructure",
    metrics: "Metrics", traces: "Traces", logs: "Logs", incidents: "Incidents", deployments: "Deployments", alertRules: "Alert rules",
    telemetryVolume: "Telemetry volume", settings: "Settings", helpSupport: "Help & support", searchServices: "Search services...",
    docs: "Docs", systemOverview: "SYSTEM OVERVIEW", apiConnected: "API CONNECTED", demoEnvironment: "DEMO ENVIRONMENT",
    goodMorning: "Good morning, team.", overviewSubtitle: "Here's what's happening across your infrastructure.",
    liveQuerySubtitle: "Query {view} from the connected observability pipeline.", connected: "Connected",
    autoRefreshPaused: "Auto-refresh paused in demo", manualRefresh: "Manual refresh mode",
    last24Hours: "Last 24 hours", exploreTopology: "Explore topology",
    sampleEnvironmentTitle: "You're viewing a sample environment.",
    sampleEnvironmentBody: " Metrics, services, and incidents are illustrative demo data — no production systems are connected.",
    connectedTo: "Connected to", connectedInventory: " Services and incidents are read from {environment}; each record keeps its own data-source label.",
    manageSource: "Manage source", connectSource: "Connect a source", connectApiTitle: "Connect an ORBIT API",
    apiHelp: "Use a read-only API endpoint. For local development, start Docker Compose and use http://localhost:8080.",
    apiBaseUrl: "API base URL", connecting: "Connecting…", testConnect: "Test and connect", switchToDemo: "Switch to demo",
    connectionVerified: "Connection verified.", servicesLoaded: "services and", incidentsLoaded: "incidents loaded.",
    connectTelemetryTitle: "Connect a telemetry source",
    connectTelemetryBody: "Live {view} queries are available after connecting an ORBIT API backed by the local observability stack. Demo fixtures are never presented as live telemetry.",
    connectApiSource: "Connect API source", keyMetrics: "Key metrics", requestsPerMinute: "REQUESTS / MIN", p95Latency: "P95 LATENCY",
    errorRate: "ERROR RATE", activeServices: "ACTIVE SERVICES", vsPrevious: "vs. previous period", degraded: "degraded",
    acrossRegions: "across 3 regions", trafficPerformance: "Traffic & performance",
    incomingRequests: "Incoming request volume across all services", sample: "SAMPLE", comparedPrior: "compared to prior period",
    requestPerMinute: "req / min", illustrativeTrend: "Illustrative request volume trend", requestsPerMin: "Requests / min",
    previousPeriod: "Previous period", illustrativeTelemetry: "Illustrative telemetry",
    serviceHealth: "Service health", monitored: "monitored", serviceHealthSubtitle: "Health across your distributed services",
    showLess: "Show less", viewAll: "View all", service: "SERVICE", latency: "LATENCY", status: "STATUS",
    noServicesMatch: "No services match", showing: "Showing", of: "of", apiData: "API DATA", demoData: "DEMO DATA",
    activeIncidents: "Active incidents", issuesAttention: "Issues that may need attention", incidentOptions: "Incident options",
    noActiveIncidents: "No active incidents reported by this source.", incidentTimeline: "View incident timeline",
    availabilitySlo: "AVAILABILITY SLO", withinTarget: "Within target", deploymentsToday: "DEPLOYMENTS TODAY",
    acrossFourServices: "Across 4 services", dataIngestion: "DATA INGESTION", sampleStream: "Sample stream",
    inventoryFromApi: "Service and incident inventory from connected API", allIllustrative: "All systems are illustrative",
    systemsObservatory: "SYSTEMS OBSERVATORY", projectOwner: "PROJECT OWNER", createdMaintained: "Created and maintained by",
    language: "Language", english: "English", persian: "فارسی", closeNavigation: "Close navigation",
    openNavigation: "Open navigation", notifications: "Notifications", searchServicesLabel: "Search services",
    refreshApi: "Refresh API data", pauseRefresh: "Pause demo refresh", resumeRefresh: "Resume demo refresh",
    refresh: "Refresh", unknown: "Unknown", operational: "Operational", incident: "Incident", unknownStatus: "Unknown",
    regionUnavailable: "region unavailable", timeUnavailable: "time unavailable", justNow: "just now", minAgo: "min ago",
    hrAgo: "hr ago", dayAgo: "d ago", invalidApiUrl: "Enter an API base URL such as http://localhost:8080.",
    validApiUrl: "Enter a valid API URL.", connectFailed: "Unable to connect to the ORBIT API."
  },
  fa: {
    workspace: "فضای کاری", production: "محیط عملیاتی", observe: "پایش", investigate: "بررسی", deliver: "انتشار",
    overview: "نمای کلی", serviceMap: "نقشهٔ سرویس‌ها", services: "سرویس‌ها", infrastructure: "زیرساخت",
    metrics: "شاخص‌ها", traces: "ردیابی‌ها", logs: "گزارش‌ها", incidents: "رخدادها", deployments: "استقرارها", alertRules: "قوانین هشدار",
    telemetryVolume: "حجم داده‌های تله‌متری", settings: "تنظیمات", helpSupport: "راهنما و پشتیبانی", searchServices: "جست‌وجوی سرویس‌ها...",
    docs: "مستندات", systemOverview: "نمای کلی سامانه", apiConnected: "اتصال API برقرار است", demoEnvironment: "محیط نمایشی",
    goodMorning: "نمای کلی سامانهٔ ORBIT", overviewSubtitle: "وضعیت سرویس‌ها و زیرساخت تو در یک نگاه.",
    liveQuerySubtitle: "دریافت داده‌های {view} از خط لولهٔ پایش متصل‌شده.", connected: "متصل",
    autoRefreshPaused: "به‌روزرسانی خودکار در حالت نمایشی متوقف است", manualRefresh: "حالت به‌روزرسانی دستی",
    last24Hours: "۲۴ ساعت گذشته", exploreTopology: "مشاهدهٔ توپولوژی",
    sampleEnvironmentTitle: "در حال مشاهدهٔ یک محیط نمونه هستی.",
    sampleEnvironmentBody: " شاخص‌ها، سرویس‌ها و رخدادها داده‌های نمایشی هستند و به هیچ سامانهٔ عملیاتی متصل نیستند.",
    connectedTo: "اتصال به", connectedInventory: " سرویس‌ها و رخدادها از {environment} خوانده می‌شوند و منبع هر رکورد جداگانه مشخص است.",
    manageSource: "مدیریت منبع", connectSource: "اتصال منبع", connectApiTitle: "اتصال به ORBIT API",
    apiHelp: "از یک نشانی فقط‌خواندنی API استفاده کن. برای اجرای محلی، Docker Compose را اجرا و نشانی http://localhost:8080 را وارد کن.",
    apiBaseUrl: "نشانی پایهٔ API", connecting: "در حال اتصال…", testConnect: "بررسی و اتصال", switchToDemo: "بازگشت به حالت نمایشی",
    connectionVerified: "اتصال با موفقیت بررسی شد.", servicesLoaded: "سرویس و", incidentsLoaded: "رخداد بارگذاری شد.",
    connectTelemetryTitle: "اتصال به منبع تله‌متری",
    connectTelemetryBody: "پس از اتصال به ORBIT API متصل به زیرساخت پایش محلی، امکان دریافت زندهٔ {view} فراهم می‌شود. داده‌های نمونه هرگز به‌عنوان دادهٔ زنده نمایش داده نمی‌شوند.",
    connectApiSource: "اتصال به منبع API", keyMetrics: "شاخص‌های کلیدی", requestsPerMinute: "تعداد درخواست در دقیقه", p95Latency: "تأخیر صدک ۹۵",
    errorRate: "نرخ خطا", activeServices: "سرویس‌های فعال", vsPrevious: "در مقایسه با بازهٔ قبل", degraded: "دارای اختلال",
    acrossRegions: "در ۳ منطقه", trafficPerformance: "ترافیک و کارایی",
    incomingRequests: "حجم درخواست‌های ورودی در تمام سرویس‌ها", sample: "نمونه", comparedPrior: "در مقایسه با بازهٔ قبل",
    requestPerMinute: "درخواست در دقیقه", illustrativeTrend: "روند نمایشی حجم درخواست‌ها", requestsPerMin: "درخواست در دقیقه",
    previousPeriod: "بازهٔ قبلی", illustrativeTelemetry: "دادهٔ تله‌متری نمایشی",
    serviceHealth: "سلامت سرویس‌ها", monitored: "مورد پایش", serviceHealthSubtitle: "وضعیت سلامت سرویس‌های توزیع‌شده",
    showLess: "نمایش کمتر", viewAll: "مشاهدهٔ همه", service: "سرویس", latency: "تأخیر", status: "وضعیت",
    noServicesMatch: "سرویسی با این عبارت پیدا نشد:", showing: "نمایش", of: "از", apiData: "دادهٔ API", demoData: "دادهٔ نمونه",
    activeIncidents: "رخدادهای فعال", issuesAttention: "مواردی که ممکن است به بررسی نیاز داشته باشند", incidentOptions: "گزینه‌های رخداد",
    noActiveIncidents: "این منبع هیچ رخداد فعالی گزارش نکرده است.", incidentTimeline: "مشاهدهٔ تاریخچهٔ رخدادها",
    availabilitySlo: "هدف سطح دسترس‌پذیری", withinTarget: "در محدودهٔ هدف", deploymentsToday: "استقرارهای امروز",
    acrossFourServices: "در ۴ سرویس", dataIngestion: "دریافت داده", sampleStream: "جریان نمونه",
    inventoryFromApi: "فهرست سرویس‌ها و رخدادها از API متصل‌شده", allIllustrative: "تمام سامانه‌ها نمایشی هستند",
    systemsObservatory: "مرکز پایش سامانه‌ها", projectOwner: "مالک پروژه", createdMaintained: "توسعه و نگه‌داری توسط",
    language: "زبان", english: "English", persian: "فارسی", closeNavigation: "بستن پیمایش",
    openNavigation: "باز کردن پیمایش", notifications: "اعلان‌ها", searchServicesLabel: "جست‌وجوی سرویس‌ها",
    refreshApi: "به‌روزرسانی داده‌های API", pauseRefresh: "توقف به‌روزرسانی نمایشی", resumeRefresh: "ادامهٔ به‌روزرسانی نمایشی",
    refresh: "به‌روزرسانی", unknown: "نامشخص", operational: "عملیاتی", incident: "دارای رخداد", unknownStatus: "نامشخص",
    regionUnavailable: "منطقه نامشخص", timeUnavailable: "زمان نامشخص", justNow: "همین الان", minAgo: "دقیقه پیش",
    hrAgo: "ساعت پیش", dayAgo: "روز پیش", invalidApiUrl: "نشانی پایهٔ API را وارد کن؛ برای نمونه http://localhost:8080.",
    validApiUrl: "یک نشانی معتبر API وارد کن.", connectFailed: "اتصال به ORBIT API ممکن نشد."
  }
} as const;

export type TranslationKey = keyof typeof messages.en;

export function t(language: Language, key: TranslationKey): string {
  return messages[language][key] ?? messages.en[key];
}

export function navLabel(language: Language, label: string): string {
  const keyByLabel: Record<string, TranslationKey> = {
    Overview: "overview", "Service map": "serviceMap", Services: "services",
    Infrastructure: "infrastructure", Metrics: "metrics", Traces: "traces",
    Logs: "logs", Incidents: "incidents", Deployments: "deployments",
    "Alert rules": "alertRules"
  };
  const key = keyByLabel[label];
  return key ? t(language, key) : label;
}

export function formatLocale(language: Language): string {
  return language === "fa" ? "fa-IR" : "en";
}
