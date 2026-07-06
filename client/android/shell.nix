# Reproducible Android build toolchain for the Capacitor shell, pinned via Nix.
#
#   nix-shell client/android/shell.nix --run './gradlew :app:assembleRelease'
#
# Capacitor 6 android pins AGP/Gradle 8.2.1 and compileSdk 34, but Google Play
# requires targetSdk 35, so both platforms and build-tools are provisioned.
# Gradle 8.2.1 does NOT run on Java 21, so JDK 17 is pinned inside the shell -
# do not rely on a system JDK.
{ pkgs ? import <nixpkgs> {
    config = {
      allowUnfree = true;
      android_sdk.accept_license = true;
    };
  }
}:
let
  android = pkgs.androidenv.composeAndroidPackages {
    platformVersions = [ "34" "35" ];
    buildToolsVersions = [ "34.0.0" "35.0.0" ];
    includeEmulator = false;
    includeSystemImages = false;
    includeNDK = false;
  };
  sdkRoot = "${android.androidsdk}/libexec/android-sdk";
in
pkgs.mkShell {
  buildInputs = [ android.androidsdk pkgs.jdk17 ];
  ANDROID_HOME = sdkRoot;
  ANDROID_SDK_ROOT = sdkRoot;
  JAVA_HOME = pkgs.jdk17.home;
  # AGP fetches aapt2 from Maven by default; point it at the SDK copy so the
  # build stays hermetic under Nix.
  GRADLE_OPTS = "-Dorg.gradle.project.android.aapt2FromMavenOverride=${sdkRoot}/build-tools/35.0.0/aapt2";
}
