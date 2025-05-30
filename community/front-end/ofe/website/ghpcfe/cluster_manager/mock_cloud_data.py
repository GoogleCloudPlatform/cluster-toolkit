"""Mock cloud data for local development environment"""

# Mock GCP regions and zones
GCP_REGIONS_ZONES = {
    'us-central1': ['us-central1-a', 'us-central1-b', 'us-central1-c'],
    'us-west1': ['us-west1-a', 'us-west1-b', 'us-west1-c'],
    'europe-west4': ['europe-west4-a', 'europe-west4-b', 'europe-west4-c'],
    'asia-east1': ['asia-east1-a', 'asia-east1-b', 'asia-east1-c']
}

# Mock GCP machine types
GCP_MACHINE_TYPES = {
    # N1 family
    'n1-standard-1': {
        'name': 'n1-standard-1',
        'description': 'Standard machine type with 1 vCPU and 3.75 GB memory',
        'family': 'n1',
        'memory': 3840,
        'vCPU': 1,
        'arch': 'x86_64'
    },
    'n1-standard-2': {
        'name': 'n1-standard-2',
        'description': 'Standard machine type with 2 vCPUs and 7.5 GB memory',
        'family': 'n1',
        'memory': 7680,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    'n1-standard-4': {
        'name': 'n1-standard-4',
        'description': 'Standard machine type with 4 vCPUs and 15 GB memory',
        'family': 'n1',
        'memory': 15360,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'n1-standard-8': {
        'name': 'n1-standard-8',
        'description': 'Standard machine type with 8 vCPUs and 30 GB memory',
        'family': 'n1',
        'memory': 30720,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    'n1-standard-16': {
        'name': 'n1-standard-16',
        'description': 'Standard machine type with 16 vCPUs and 60 GB memory',
        'family': 'n1',
        'memory': 61440,
        'vCPU': 16,
        'arch': 'x86_64'
    },
    'n1-highmem-2': {
        'name': 'n1-highmem-2',
        'description': 'High-memory machine type with 2 vCPUs and 13 GB memory',
        'family': 'n1',
        'memory': 13312,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    'n1-highmem-4': {
        'name': 'n1-highmem-4',
        'description': 'High-memory machine type with 4 vCPUs and 26 GB memory',
        'family': 'n1',
        'memory': 26624,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'n1-highcpu-16': {
        'name': 'n1-highcpu-16',
        'description': 'High-CPU machine type with 16 vCPUs and 14.4 GB memory',
        'family': 'n1',
        'memory': 14746,
        'vCPU': 16,
        'arch': 'x86_64'
    },

    # N2 family
    'n2-standard-2': {
        'name': 'n2-standard-2',
        'description': 'N2 standard machine type with 2 vCPUs and 8 GB memory',
        'family': 'n2',
        'memory': 8192,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    'n2-standard-4': {
        'name': 'n2-standard-4',
        'description': 'N2 standard machine type with 4 vCPUs and 16 GB memory',
        'family': 'n2',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'n2-standard-8': {
        'name': 'n2-standard-8',
        'description': 'N2 standard machine type with 8 vCPUs and 32 GB memory',
        'family': 'n2',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    'n2-standard-16': {
        'name': 'n2-standard-16',
        'description': 'N2 standard machine type with 16 vCPUs and 64 GB memory',
        'family': 'n2',
        'memory': 65536,
        'vCPU': 16,
        'arch': 'x86_64'
    },
    'n2-highmem-2': {
        'name': 'n2-highmem-2',
        'description': 'N2 high-memory machine type with 2 vCPUs and 16 GB memory',
        'family': 'n2',
        'memory': 16384,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    'n2-highmem-4': {
        'name': 'n2-highmem-4',
        'description': 'N2 high-memory machine type with 4 vCPUs and 32 GB memory',
        'family': 'n2',
        'memory': 32768,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'n2-highcpu-16': {
        'name': 'n2-highcpu-16',
        'description': 'N2 high-CPU machine type with 16 vCPUs and 16 GB memory',
        'family': 'n2',
        'memory': 16384,
        'vCPU': 16,
        'arch': 'x86_64'
    },
    'n2-highcpu-32': {
        'name': 'n2-highcpu-32',
        'description': 'N2 high-CPU machine type with 32 vCPUs and 32 GB memory',
        'family': 'n2',
        'memory': 32768,
        'vCPU': 32,
        'arch': 'x86_64'
    },

    # E2 family
    'e2-micro': {
        'name': 'e2-micro',
        'description': 'E2 micro machine type with 0.25-2 vCPUs and 1 GB memory',
        'family': 'e2',
        'memory': 1024,
        'vCPU': 1,
        'arch': 'x86_64'
    },
    'e2-small': {
        'name': 'e2-small',
        'description': 'E2 small machine type with 0.5-2 vCPUs and 2 GB memory',
        'family': 'e2',
        'memory': 2048,
        'vCPU': 1,
        'arch': 'x86_64'
    },
    'e2-medium': {
        'name': 'e2-medium',
        'description': 'E2 medium machine type with 1-2 vCPUs and 4 GB memory',
        'family': 'e2',
        'memory': 4096,
        'vCPU': 1,
        'arch': 'x86_64'
    },
    'e2-standard-2': {
        'name': 'e2-standard-2',
        'description': 'E2 standard machine type with 2 vCPUs and 8 GB memory',
        'family': 'e2',
        'memory': 8192,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    'e2-standard-4': {
        'name': 'e2-standard-4',
        'description': 'E2 standard machine type with 4 vCPUs and 16 GB memory',
        'family': 'e2',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'e2-standard-8': {
        'name': 'e2-standard-8',
        'description': 'E2 standard machine type with 8 vCPUs and 32 GB memory',
        'family': 'e2',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    'e2-highmem-2': {
        'name': 'e2-highmem-2',
        'description': 'E2 high-memory machine type with 2 vCPUs and 16 GB memory',
        'family': 'e2',
        'memory': 16384,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    'e2-highmem-4': {
        'name': 'e2-highmem-4',
        'description': 'E2 high-memory machine type with 4 vCPUs and 32 GB memory',
        'family': 'e2',
        'memory': 32768,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'e2-highcpu-16': {
        'name': 'e2-highcpu-16',
        'description': 'E2 high-CPU machine type with 16 vCPUs and 16 GB memory',
        'family': 'e2',
        'memory': 16384,
        'vCPU': 16,
        'arch': 'x86_64'
    },

    # C2 family
    'c2-standard-4': {
        'name': 'c2-standard-4',
        'description': 'C2 compute-optimized machine type with 4 vCPUs and 16 GB memory',
        'family': 'c2',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'c2-standard-8': {
        'name': 'c2-standard-8',
        'description': 'C2 compute-optimized machine type with 8 vCPUs and 32 GB memory',
        'family': 'c2',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    'c2-standard-16': {
        'name': 'c2-standard-16',
        'description': 'C2 compute-optimized machine type with 16 vCPUs and 64 GB memory',
        'family': 'c2',
        'memory': 65536,
        'vCPU': 16,
        'arch': 'x86_64'
    },
    'c2-standard-30': {
        'name': 'c2-standard-30',
        'description': 'C2 compute-optimized machine type with 30 vCPUs and 120 GB memory',
        'family': 'c2',
        'memory': 122880,
        'vCPU': 30,
        'arch': 'x86_64'
    },
    'c2-standard-60': {
        'name': 'c2-standard-60',
        'description': 'C2 compute-optimized machine type with 60 vCPUs and 240 GB memory',
        'family': 'c2',
        'memory': 245760,
        'vCPU': 60,
        'arch': 'x86_64'
    },

    # M1 family
    'm1-ultramem-40': {
        'name': 'm1-ultramem-40',
        'description': 'M1 ultra high-memory machine type with 40 vCPUs and 961 GB memory',
        'family': 'm1',
        'memory': 984064,
        'vCPU': 40,
        'arch': 'x86_64'
    },
    'm1-ultramem-80': {
        'name': 'm1-ultramem-80',
        'description': 'M1 ultra high-memory machine type with 80 vCPUs and 1922 GB memory',
        'family': 'm1',
        'memory': 1968128,
        'vCPU': 80,
        'arch': 'x86_64'
    },
    'm1-ultramem-160': {
        'name': 'm1-ultramem-160',
        'description': 'M1 ultra high-memory machine type with 160 vCPUs and 3844 GB memory',
        'family': 'm1',
        'memory': 3936256,
        'vCPU': 160,
        'arch': 'x86_64'
    },
    'm1-megamem-96': {
        'name': 'm1-megamem-96',
        'description': 'M1 mega high-memory machine type with 96 vCPUs and 1433.6 GB memory',
        'family': 'm1',
        'memory': 1468006,
        'vCPU': 96,
        'arch': 'x86_64'
    },

    # T2D family
    't2d-standard-1': {
        'name': 't2d-standard-1',
        'description': 'T2D standard machine type with 1 vCPU and 4 GB memory (AMD)',
        'family': 't2d',
        'memory': 4096,
        'vCPU': 1,
        'arch': 'x86_64'
    },
    't2d-standard-2': {
        'name': 't2d-standard-2',
        'description': 'T2D standard machine type with 2 vCPUs and 8 GB memory (AMD)',
        'family': 't2d',
        'memory': 8192,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    't2d-standard-4': {
        'name': 't2d-standard-4',
        'description': 'T2D standard machine type with 4 vCPUs and 16 GB memory (AMD)',
        'family': 't2d',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    't2d-standard-8': {
        'name': 't2d-standard-8',
        'description': 'T2D standard machine type with 8 vCPUs and 32 GB memory (AMD)',
        'family': 't2d',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    't2d-standard-16': {
        'name': 't2d-standard-16',
        'description': 'T2D standard machine type with 16 vCPUs and 64 GB memory (AMD)',
        'family': 't2d',
        'memory': 65536,
        'vCPU': 16,
        'arch': 'x86_64'
    },

    # A2 family
    'a2-highgpu-1g': {
        'name': 'a2-highgpu-1g',
        'description': 'A2 accelerator-optimized machine type with 12 vCPUs, 85 GB memory and 1 A100 GPU',
        'family': 'a2',
        'memory': 87040,
        'vCPU': 12,
        'arch': 'x86_64'
    },
    'a2-highgpu-2g': {
        'name': 'a2-highgpu-2g',
        'description': 'A2 accelerator-optimized machine type with 24 vCPUs, 170 GB memory and 2 A100 GPUs',
        'family': 'a2',
        'memory': 174080,
        'vCPU': 24,
        'arch': 'x86_64'
    },
    'a2-highgpu-4g': {
        'name': 'a2-highgpu-4g',
        'description': 'A2 accelerator-optimized machine type with 48 vCPUs, 340 GB memory and 4 A100 GPUs',
        'family': 'a2',
        'memory': 348160,
        'vCPU': 48,
        'arch': 'x86_64'
    },

    # G2 family
    'g2-standard-4': {
        'name': 'g2-standard-4',
        'description': 'G2 GPU machine type with 4 vCPUs, 16 GB memory and 1 L4 GPU',
        'family': 'g2',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'g2-standard-8': {
        'name': 'g2-standard-8',
        'description': 'G2 GPU machine type with 8 vCPUs, 32 GB memory and 1 L4 GPU',
        'family': 'g2',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    'g2-standard-12': {
        'name': 'g2-standard-12',
        'description': 'G2 GPU machine type with 12 vCPUs, 48 GB memory and 1 L4 GPU',
        'family': 'g2',
        'memory': 49152,
        'vCPU': 12,
        'arch': 'x86_64'
    },

    # A3 family
    'a3-highgpu-8g': {
        'name': 'a3-highgpu-8g',
        'description': 'A3 accelerator-optimized machine type with 208 vCPUs, 1872 GB memory and 8 H100 GPUs',
        'family': 'a3',
        'memory': 1916928,
        'vCPU': 208,
        'arch': 'x86_64'
    },
    'a3-megagpu-8g': {
        'name': 'a3-megagpu-8g',
        'description': 'A3 mega accelerator-optimized machine type with 208 vCPUs, 1872 GB memory and 8 H100 GPUs',
        'family': 'a3',
        'memory': 1916928,
        'vCPU': 208,
        'arch': 'x86_64'
    },

    # C3 family
    'c3-standard-4': {
        'name': 'c3-standard-4',
        'description': 'C3 compute-optimized machine type with 4 vCPUs and 16 GB memory',
        'family': 'c3',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'c3-standard-8': {
        'name': 'c3-standard-8',
        'description': 'C3 compute-optimized machine type with 8 vCPUs and 32 GB memory',
        'family': 'c3',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    'c3-standard-22': {
        'name': 'c3-standard-22',
        'description': 'C3 compute-optimized machine type with 22 vCPUs and 88 GB memory',
        'family': 'c3',
        'memory': 90112,
        'vCPU': 22,
        'arch': 'x86_64'
    },
    'c3-standard-44': {
        'name': 'c3-standard-44',
        'description': 'C3 compute-optimized machine type with 44 vCPUs and 176 GB memory',
        'family': 'c3',
        'memory': 180224,
        'vCPU': 44,
        'arch': 'x86_64'
    },
    'c3-standard-88': {
        'name': 'c3-standard-88',
        'description': 'C3 compute-optimized machine type with 88 vCPUs and 352 GB memory',
        'family': 'c3',
        'memory': 360448,
        'vCPU': 88,
        'arch': 'x86_64'
    },
    'c3-standard-176': {
        'name': 'c3-standard-176',
        'description': 'C3 compute-optimized machine type with 176 vCPUs and 704 GB memory',
        'family': 'c3',
        'memory': 720896,
        'vCPU': 176,
        'arch': 'x86_64'
    },
    'c3-highcpu-88': {
        'name': 'c3-highcpu-88',
        'description': 'C3 high-CPU machine type with 88 vCPUs and 88 GB memory',
        'family': 'c3',
        'memory': 90112,
        'vCPU': 88,
        'arch': 'x86_64'
    },
    'c3-highcpu-176': {
        'name': 'c3-highcpu-176',
        'description': 'C3 high-CPU machine type with 176 vCPUs and 176 GB memory',
        'family': 'c3',
        'memory': 180224,
        'vCPU': 176,
        'arch': 'x86_64'
    },

    # C3D family
    'c3d-standard-4': {
        'name': 'c3d-standard-4',
        'description': 'C3D AMD compute-optimized machine type with 4 vCPUs and 16 GB memory',
        'family': 'c3d',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'c3d-standard-8': {
        'name': 'c3d-standard-8',
        'description': 'C3D AMD compute-optimized machine type with 8 vCPUs and 32 GB memory',
        'family': 'c3d',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    'c3d-standard-30': {
        'name': 'c3d-standard-30',
        'description': 'C3D AMD compute-optimized machine type with 30 vCPUs and 120 GB memory',
        'family': 'c3d',
        'memory': 122880,
        'vCPU': 30,
        'arch': 'x86_64'
    },
    'c3d-standard-60': {
        'name': 'c3d-standard-60',
        'description': 'C3D AMD compute-optimized machine type with 60 vCPUs and 240 GB memory',
        'family': 'c3d',
        'memory': 245760,
        'vCPU': 60,
        'arch': 'x86_64'
    },
    'c3d-standard-90': {
        'name': 'c3d-standard-90',
        'description': 'C3D AMD compute-optimized machine type with 90 vCPUs and 360 GB memory',
        'family': 'c3d',
        'memory': 368640,
        'vCPU': 90,
        'arch': 'x86_64'
    },
    'c3d-highcpu-30': {
        'name': 'c3d-highcpu-30',
        'description': 'C3D AMD high-CPU machine type with 30 vCPUs and 30 GB memory',
        'family': 'c3d',
        'memory': 30720,
        'vCPU': 30,
        'arch': 'x86_64'
    },
    'c3d-highcpu-60': {
        'name': 'c3d-highcpu-60',
        'description': 'C3D AMD high-CPU machine type with 60 vCPUs and 60 GB memory',
        'family': 'c3d',
        'memory': 61440,
        'vCPU': 60,
        'arch': 'x86_64'
    },

    # N2D family
    'n2d-standard-2': {
        'name': 'n2d-standard-2',
        'description': 'N2D AMD standard machine type with 2 vCPUs and 8 GB memory',
        'family': 'n2d',
        'memory': 8192,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    'n2d-standard-4': {
        'name': 'n2d-standard-4',
        'description': 'N2D AMD standard machine type with 4 vCPUs and 16 GB memory',
        'family': 'n2d',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'n2d-standard-8': {
        'name': 'n2d-standard-8',
        'description': 'N2D AMD standard machine type with 8 vCPUs and 32 GB memory',
        'family': 'n2d',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'x86_64'
    },
    'n2d-standard-16': {
        'name': 'n2d-standard-16',
        'description': 'N2D AMD standard machine type with 16 vCPUs and 64 GB memory',
        'family': 'n2d',
        'memory': 65536,
        'vCPU': 16,
        'arch': 'x86_64'
    },
    'n2d-standard-32': {
        'name': 'n2d-standard-32',
        'description': 'N2D AMD standard machine type with 32 vCPUs and 128 GB memory',
        'family': 'n2d',
        'memory': 131072,
        'vCPU': 32,
        'arch': 'x86_64'
    },
    'n2d-highmem-2': {
        'name': 'n2d-highmem-2',
        'description': 'N2D AMD high-memory machine type with 2 vCPUs and 16 GB memory',
        'family': 'n2d',
        'memory': 16384,
        'vCPU': 2,
        'arch': 'x86_64'
    },
    'n2d-highmem-4': {
        'name': 'n2d-highmem-4',
        'description': 'N2D AMD high-memory machine type with 4 vCPUs and 32 GB memory',
        'family': 'n2d',
        'memory': 32768,
        'vCPU': 4,
        'arch': 'x86_64'
    },
    'n2d-highcpu-16': {
        'name': 'n2d-highcpu-16',
        'description': 'N2D AMD high-CPU machine type with 16 vCPUs and 16 GB memory',
        'family': 'n2d',
        'memory': 16384,
        'vCPU': 16,
        'arch': 'x86_64'
    },
    'n2d-highcpu-32': {
        'name': 'n2d-highcpu-32',
        'description': 'N2D AMD high-CPU machine type with 32 vCPUs and 32 GB memory',
        'family': 'n2d',
        'memory': 32768,
        'vCPU': 32,
        'arch': 'x86_64'
    },

    # M2 family
    'm2-ultramem-208': {
        'name': 'm2-ultramem-208',
        'description': 'M2 ultra high-memory machine type with 208 vCPUs and 5888 GB memory',
        'family': 'm2',
        'memory': 6029312,
        'vCPU': 208,
        'arch': 'x86_64'
    },
    'm2-ultramem-416': {
        'name': 'm2-ultramem-416',
        'description': 'M2 ultra high-memory machine type with 416 vCPUs and 11776 GB memory',
        'family': 'm2',
        'memory': 12058624,
        'vCPU': 416,
        'arch': 'x86_64'
    },
    'm2-megamem-416': {
        'name': 'm2-megamem-416',
        'description': 'M2 mega high-memory machine type with 416 vCPUs and 5888 GB memory',
        'family': 'm2',
        'memory': 6029312,
        'vCPU': 416,
        'arch': 'x86_64'
    },

    # M3 family
    'm3-ultramem-32': {
        'name': 'm3-ultramem-32',
        'description': 'M3 ultra high-memory machine type with 32 vCPUs and 976 GB memory',
        'family': 'm3',
        'memory': 999424,
        'vCPU': 32,
        'arch': 'x86_64'
    },
    'm3-ultramem-64': {
        'name': 'm3-ultramem-64',
        'description': 'M3 ultra high-memory machine type with 64 vCPUs and 1952 GB memory',
        'family': 'm3',
        'memory': 1998848,
        'vCPU': 64,
        'arch': 'x86_64'
    },
    'm3-ultramem-128': {
        'name': 'm3-ultramem-128',
        'description': 'M3 ultra high-memory machine type with 128 vCPUs and 3904 GB memory',
        'family': 'm3',
        'memory': 3997696,
        'vCPU': 128,
        'arch': 'x86_64'
    },
    'm3-megamem-64': {
        'name': 'm3-megamem-64',
        'description': 'M3 mega high-memory machine type with 64 vCPUs and 976 GB memory',
        'family': 'm3',
        'memory': 999424,
        'vCPU': 64,
        'arch': 'x86_64'
    },
    'm3-megamem-128': {
        'name': 'm3-megamem-128',
        'description': 'M3 mega high-memory machine type with 128 vCPUs and 1952 GB memory',
        'family': 'm3',
        'memory': 1998848,
        'vCPU': 128,
        'arch': 'x86_64'
    },

    # T2A family
    't2a-standard-1': {
        'name': 't2a-standard-1',
        'description': 'T2A ARM standard machine type with 1 vCPU and 4 GB memory',
        'family': 't2a',
        'memory': 4096,
        'vCPU': 1,
        'arch': 'aarch64'
    },
    't2a-standard-2': {
        'name': 't2a-standard-2',
        'description': 'T2A ARM standard machine type with 2 vCPUs and 8 GB memory',
        'family': 't2a',
        'memory': 8192,
        'vCPU': 2,
        'arch': 'aarch64'
    },
    't2a-standard-4': {
        'name': 't2a-standard-4',
        'description': 'T2A ARM standard machine type with 4 vCPUs and 16 GB memory',
        'family': 't2a',
        'memory': 16384,
        'vCPU': 4,
        'arch': 'aarch64'
    },
    't2a-standard-8': {
        'name': 't2a-standard-8',
        'description': 'T2A ARM standard machine type with 8 vCPUs and 32 GB memory',
        'family': 't2a',
        'memory': 32768,
        'vCPU': 8,
        'arch': 'aarch64'
    },
    't2a-standard-16': {
        'name': 't2a-standard-16',
        'description': 'T2A ARM standard machine type with 16 vCPUs and 64 GB memory',
        'family': 't2a',
        'memory': 65536,
        'vCPU': 16,
        'arch': 'aarch64'
    },
    't2a-standard-32': {
        'name': 't2a-standard-32',
        'description': 'T2A ARM standard machine type with 32 vCPUs and 128 GB memory',
        'family': 't2a',
        'memory': 131072,
        'vCPU': 32,
        'arch': 'aarch64'
    },
    't2a-standard-48': {
        'name': 't2a-standard-48',
        'description': 'T2A ARM standard machine type with 48 vCPUs and 192 GB memory',
        'family': 't2a',
        'memory': 196608,
        'vCPU': 48,
        'arch': 'aarch64'
    },
}

# Mock GCP subnets
GCP_SUBNETS = [
    # [vpc_name, region, subnet_name, cidr]
    ['default', 'us-central1', 'default-us-central1', '10.128.0.0/20'],
    ['default', 'us-west1', 'default-us-west1', '10.138.0.0/20'],
    ['default', 'europe-west4', 'default-europe-west4', '10.164.0.0/20']
]

# Mock GCP disk types
GCP_DISK_TYPES = [
    {
        'description': 'Standard persistent disk',
        'name': 'pd-standard',
        'minSizeGB': 10,
        'maxSizeGB': 65536
    },
    {
        'description': 'SSD persistent disk',
        'name': 'pd-ssd',
        'minSizeGB': 10,
        'maxSizeGB': 65536
    },
    {
        'description': 'Balanced persistent disk',
        'name': 'pd-balanced',
        'minSizeGB': 10,
        'maxSizeGB': 65536
    }
]

# GPU accelerators 
A2_ACCELERATORS = {
    'nvidia-tesla-a100': {
        'description': 'NVIDIA Tesla A100',
        'min_count': 1,
        'max_count': 16
    }
}

A3_ACCELERATORS = {
    'nvidia-h100': {
        'description': 'NVIDIA H100',
        'min_count': 8,
        'max_count': 8
    }
}

G2_ACCELERATORS = {
    'nvidia-l4': {
        'description': 'NVIDIA L4',
        'min_count': 1,
        'max_count': 8
    }
}

# Add accelerators to specific machine types
for machine in GCP_MACHINE_TYPES.values():
    if machine['family'] == 'a2':
        machine['accelerators'] = A2_ACCELERATORS
    elif machine['family'] == 'a3':
        machine['accelerators'] = A3_ACCELERATORS
    elif machine['family'] == 'g2':
        machine['accelerators'] = G2_ACCELERATORS
    else:
        machine['accelerators'] = {} 