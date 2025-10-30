// ui/app.js
const { createApp, ref, computed, watch } = Vue;

const app = createApp({
    setup() {
        const cameras = ref([]);
        const selectedCam = ref('');
        const available = ref([]);
        const selectedDate = ref('');
        const selectedSeg = ref('');
        const existingSegments = ref([]);
        const hoverIndex = ref(-1);
        const previewLeft = ref(0);
        const previewTop = ref(0);
        const videoRef = ref(null);

        const selectedColor = computed(() => {
            const cam = cameras.value.find(c => c.id === selectedCam.value);
            return cam ? cam.color : '#000000';
        });

        const videoSrc = computed(() => {
            if (!selectedSeg.value) return '';
            return `/videos/${selectedCam.value}/${selectedDate.value}/${selectedSeg.value}/stream.m3u8`;
        });

        const thumbSrc = computed(() => {
            if (hoverIndex.value < 0) return '';
            const num = hoverIndex.value.toString().padStart(3, '0');
            return `/videos/${selectedCam.value}/${selectedDate.value}/${selectedSeg.value}/thumbnails/${num}.jpg`;
        });

        const segmentColors = computed(() => {
            const total = 2160; // 6 hours / 10s
            let colors = new Array(total).fill('#dddddd'); // gray for missing
            existingSegments.value.forEach(i => {
                if (i < total) colors[i] = selectedColor.value;
            });
            return colors;
        });

        const loadCameras = async () => {
            const res = await fetch('/api/cameras');
            cameras.value = await res.json();
        };

        const loadAvailable = async () => {
            if (!selectedCam.value) return;
            const res = await fetch(`/api/camera/${selectedCam.value}`);
            available.value = await res.json();
        };

        const selectSegment = async (date, seg) => {
            selectedDate.value = date;
            selectedSeg.value = seg;
            const res = await fetch(`/api/camera/${selectedCam.value}/${date}/${seg}/existing_segments`);
            existingSegments.value = await res.json();
        };

        const showThumb = (i, event) => {
            hoverIndex.value = i;
            previewLeft.value = event.pageX - 100;
            previewTop.value = event.pageY - 250; // Adjust as needed
        };

        const hideThumb = () => {
            hoverIndex.value = -1;
        };

        const seekTo = (i) => {
            const video = videoRef.value;
            if (video) {
                video.currentTime = i * 10;
            }
        };

        watch(videoSrc, (newSrc) => {
            if (!newSrc) return;
            const video = videoRef.value;
            if (Hls.isSupported()) {
                const hls = new Hls();
                hls.loadSource(newSrc);
                hls.attachMedia(video);
            } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
                video.src = newSrc;
            }
        });

        loadCameras();

        return {
            cameras,
            selectedCam,
            available,
            selectedDate,
            selectedSeg,
            videoRef,
            segmentColors,
            hoverIndex,
            previewLeft,
            previewTop,
            selectedColor,
            videoSrc,
            thumbSrc,
            loadAvailable,
            selectSegment,
            showThumb,
            hideThumb,
            seekTo,
        };
    }
});
app.mount('#app');