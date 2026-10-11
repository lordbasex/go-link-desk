# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
# libwebrtc calls back into these Java classes from native code.
-keep class org.webrtc.** { *; }
-dontwarn org.webrtc.**
# OkHttp's optional platform integrations.
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**
