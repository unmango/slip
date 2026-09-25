// Imported rather than written out below, because `java` on its own resolves to
// Gradle's own java extension and not to the package.
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
}

// The SDK levels live beside this file rather than in it, because the flake
// reads them too: nix/shells/android.nix composes exactly these platforms and
// build tools, and AGP responds to a missing one by trying to install it into
// the read-only nix store.
val sdk =
    Properties().apply {
        rootProject.file("sdk-versions.properties").inputStream().use { load(it) }
    }

android {
    namespace = "dev.unmango.slip"
    compileSdk = sdk.getProperty("compileSdk").toInt()
    buildToolsVersion = sdk.getProperty("buildTools")

    defaultConfig {
        applicationId = "dev.unmango.slip"
        minSdk = sdk.getProperty("minSdk").toInt()
        targetSdk = sdk.getProperty("targetSdk").toInt()
        versionCode = 1
        versionName = "0.1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        // emulator-test.sh passes the git daemon it started. Without it the
        // tests that publish skip rather than fail, so a bare
        // `gradle connectedDebugAndroidTest` still says something useful.
        testInstrumentationRunnerArguments["slipRemote"] =
            (project.findProperty("slip.remote") ?: "").toString()
    }

    buildTypes {
        release {
            isMinifyEnabled = false
        }
    }

    // One APK per ABI. The bulk of this app is libgojni.so, so a universal
    // build is four copies of the Go runtime and three of them can never run.
    // Splitting costs nothing but extra outputs, and installDebug still picks
    // the right one for whatever is plugged in.
    //
    // 32-bit x86 is left out: no phone ships it and no current emulator image
    // needs it. x86_64 stays, because the emulator does.
    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a", "x86_64")
            isUniversalApk = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }
}

dependencies {
    // Built by `make bind` at the repository root. It is not checked in: it is
    // 37M of compiled Go, and it is reproduced from the source beside it.
    implementation(files(rootProject.file("../mobile/slip.aar")))

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.compose.material3)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.security.crypto)
    implementation(libs.androidx.work.runtime.ktx)

    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.junit)
}
