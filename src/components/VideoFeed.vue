<script>
import Slider from 'vue3-slider';

export default {
    props: {
        localVideo: Boolean,  // is our local webcam (not other's camera)
        poppedOut: Boolean,   // Video is popped-out and draggable
        username: String,     // username related to this video
        isExplicit: Boolean,  // camera is marked Explicit
        isMuted: Boolean,     // camera is muted on our end
        isSourceMuted: Boolean, // camera is muted on the broadcaster's end
        isWatchingMe: Boolean, // other video is watching us back
        isFrozen: Boolean,     // video is detected as frozen
        isSpeaking: Boolean,   // video is registering audio
        watermarkImage: Image, // watermark image to overlay (nullable)
        isNoWatermark: Boolean, // camera does not want watermark protection
    },
    components: {
        Slider,
    },
    data() {
        return {
            // Volume slider
            volume: 100,

            // Volume change debounce
            volumeDebounce: null,

            // Mouse over status
            mouseOver: false,

            // Mutation Observer, to detect if the user deletes the QR code via dev tools.
            observer: null,
        };
    },
    mounted() {
        this.initWatermarkObserver();
    },
    beforeUnmount() {
        this.observer?.disconnect();
    },
    computed: {
        containerID() {
            return this.videoID + '-container';
        },
        videoID() {
            return this.localVideo ? 'localVideo' : `videofeed-${this.username}`;
        },
        textColorClass() {
            return this.isExplicit ? 'has-text-camera-red' : 'has-text-camera-blue';
        },
        muteButtonClass() {
            let classList = [
                'button is-small ml-1 p-2',
            ]

            if (this.isMuted) {
                classList.push('is-danger');
            } else {
                classList.push('is-success is-outlined');
            }

            if (!this.mouseOver) {
                classList.push('seethru');
            }

            return classList.join(' ');
        },
        muteIconClass() {
            if (this.localVideo) {
                return this.isMuted ? 'fa-microphone-slash' : 'fa-microphone';
            }
            return this.isMuted ? 'fa-volume-xmark' : 'fa-volume-high';
        }
    },
    methods: {
        closeVideo() {
            // Note: closeVideo only available for OTHER peoples cameras.
            // Closes the WebRTC connection as the offerer.
            this.$emit('close-video', this.username, 'offerer');
        },

        reopenVideo() {
            // Note: goes into openVideo(username, force)
            this.$emit('reopen-video', this.username, true);
        },

        openProfile() {
            this.$emit('open-profile', this.username);
        },

        // Toggle the Mute button
        muteVideo() {
            this.$emit('mute-video', this.username);
        },

        popoutVideo() {
            this.$emit('popout', this.username);
        },

        fullscreen(force=false) {
            // If we are popped-out, pop back in before full screen.
            if (this.poppedOut && !force) {
                this.popoutVideo();
                window.requestAnimationFrame(() => {
                    this.fullscreen(true);
                });
                return;
            }

            let $elem = document.getElementById(this.containerID);
            if ($elem) {
                if (document.fullscreenElement) {
                    document.exitFullscreen();
                } else if ($elem.requestFullscreen) {
                    $elem.requestFullscreen();
                } else {
                    window.alert("Fullscreen not supported by your browser.");
                }
            }
        },

        volumeChanged() {
            if (this.volumeDebounce !== null) {
                clearTimeout(this.volumeDebounce);
            }
            this.volumeDebounce = setTimeout(() => {
                this.$emit('set-volume', this.username, this.volume);
            }, 200);
        },

        // Show info about the watermark when the corner one is clicked.
        showWatermarkInfo() {
            this.$emit('modal-alert', {
                icon: "fa-solid fa-qrcode",
                title: "What are the QR codes on webcams?",
                message: "This QR code is a safety feature to deter people from wanting to screen record webcams on chat. " +
                    "The QR code contains the current viewer's username, the website's name, and the current date/time. The " +
                    "idea is that if a webcam is recorded and then leaked online, the QR code would connect it directly back to who exactly " +
                    "recorded it, and when.\n\n" +
                    "Notice: if you do not care about this risk, you may remove the QR code from *your own* video so that it doesn't get " +
                    "in the way of the action when other people watch your camera. This setting can be found in the Chat Settings 'Webcam' tab.",
                backgroundDismissable: true,
            });
        },

        // Mutation Observer to detect if the watermark is tampered with by the viewer.
        initWatermarkObserver() {
            const $container = this.$refs.videoContainer,
                $watermark = this.$refs.watermarkImage,
                $smallImage = this.$refs.watermarkSmallImage;

            if (!$container || !$watermark || !$smallImage) return;

            this.observer = new MutationObserver(() => {
                const stillPresent = $container.contains($watermark) && $container.contains($smallImage);
                if (!stillPresent) {
                    this.onWatermarkRemoved();
                }
            });

            this.observer.observe($container, {
                childList: true,
                subtree: true,
            });
        },
        onWatermarkRemoved() {
            this.$emit('watermark-removed', this.username);
        },
    }
}
</script>

<template>
    <div class="feed" ref="videoContainer" :id="containerID" :class="{
        'popped-out': poppedOut,
        'popped-in': !poppedOut,
    }" @mouseover="mouseOver = true" @mouseleave="mouseOver = false">
        <video
            :id="videoID"
            autoplay
            disablepictureinpicture
            playsinline
            oncontextmenu="return false;"
            :muted="localVideo"></video>

        <!-- Watermark layer -->
        <div :class="{'is-no-watermark': isNoWatermark}">
            <img :src="watermarkImage" class="watermark" ref="watermarkImage" oncontextmenu="return false">
            <img :src="watermarkImage" class="corner-watermark seethru invert-color" ref="watermarkSmallImage" @click="showWatermarkInfo()" oncontextmenu="return false">
        </div>

        <!-- Caption -->
        <div class="caption" :class="textColorClass">
            <i class="fa fa-microphone-slash mr-1 has-text-grey" v-if="isSourceMuted"></i>
            <a href="#" @click.prevent="openProfile" :class="textColorClass">{{ username }}</a>
            <i class="fa fa-people-arrows ml-1 has-text-grey is-size-7" :title="username + ' is watching your camera too'"
                v-if="isWatchingMe"></i>

            <!-- Frozen stream detection -->
            <a class="fa fa-mountain ml-1" href="#" v-if="!localVideo && isFrozen" style="color: #00FFFF"
                @click.prevent="reopenVideo()" title="Frozen video detected!"></a>

            <!-- Is speaking -->
            <span v-if="isSpeaking" class="ml-1" title="Speaking">
                <i class="fa fa-volume-high has-text-info"></i>
            </span>
        </div>

        <!-- Close button (others' videos only) -->
        <div class="close" v-if="!localVideo" :class="{'seethru': !mouseOver}">
            <a href="#" class="button is-small is-danger is-outlined px-2" title="Close video" @click.prevent="closeVideo()">
                <i class="fa fa-close"></i>
            </a>
        </div>

        <!-- Controls -->
        <div class="controls">
            <!-- Mute Button -->
            <button type="button" :class="muteButtonClass"
                @click="muteVideo()">
                <i class="fa" :class="muteIconClass"></i>
            </button>

            <!-- Pop-out Video -->
            <button type="button" class="button is-small is-light is-outlined p-2 ml-2" title="Pop out"
                :class="{'seethru': !mouseOver}"
                @click="popoutVideo()">
                <i class="fa fa-up-right-from-square"></i>
            </button>

            <!-- Full screen. -->
            <button type="button" class="button is-small is-light is-outlined p-2 ml-2" title="Go full screen"
                :class="{'seethru': !mouseOver}"
                @click="fullscreen()">
                <i class="fa fa-expand"></i>
            </button>
        </div>

        <!-- Volume slider -->
        <div class="volume-slider" v-show="!localVideo && !isMuted && mouseOver">
            <Slider v-model="volume" color="#00FF00" track-color="#006600" :min="0" :max="100" :step="1" :height="7"
                orientation="vertical" @change="volumeChanged">

            </Slider>
        </div>
    </div>
</template>

<style scoped>
.volume-slider {
    position: absolute;
    left: 18px;
    top: 30px;
    bottom: 44px;
}

/* A background image behind video elements in case they don't load properly */
video {
    background-image: url(/static/img/connection-error.png);
    background-position: center center;
    background-repeat: no-repeat;
}

/* Translucent controls until mouse over */
.seethru {
    opacity: 0.4;
}

/* Watermark image */
.is-no-watermark {
    display: none;
}
.watermark {
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    margin: auto;
    width: 40%;
    height: 40%;
    opacity: 0.02;
    animation-name: subtle-pulsate;
    animation-duration: 20s;
    animation-iteration-count: infinite;
}
.corner-watermark {
    position: absolute;
    right: 4px;
    bottom: 4px;
    width: 20%;
    min-width: 32px;
    min-height: 32px;
    max-height: 20%;
    cursor: pointer;
}
.invert-color {
    filter: invert(100%);
}

/* Animate the primary watermark to pulsate in opacity */
@keyframes subtle-pulsate {
    0% { opacity: 0.02; filter: invert(0%); }
    25% { opacity: 0.05; }
    50% { opacity: 0.02; }
    75% { opacity: 0.05; }
    90% { opacity: 0.06 }
    91% { opacity: 0.08; filter: invert(0%); }
    95% { opacity: 0.08; filter: invert(100%); }
    99% { opacity: 0.06; filter: invert(0%); }
    100% { opacity: 0.02; }
}
</style>
