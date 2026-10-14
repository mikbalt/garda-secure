package id.co.garda.tablet

import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.view.WindowManager
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        // Guard data must not appear in screenshots or the recent-apps
        // preview of the shared tablet (MOB-11).
        window.setFlags(WindowManager.LayoutParams.FLAG_SECURE, WindowManager.LayoutParams.FLAG_SECURE)
        super.onCreate(savedInstanceState)
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        // App version for the health beacon (MOB-7), without a plugin.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "securepatrol/app").setMethodCallHandler { call, result ->
            if (call.method == "version") {
                try {
                    @Suppress("DEPRECATION")
                    val info = packageManager.getPackageInfo(packageName, 0)
                    val code = if (Build.VERSION.SDK_INT >= 28) {
                        info.longVersionCode
                    } else {
                        @Suppress("DEPRECATION")
                        info.versionCode.toLong()
                    }
                    result.success(mapOf("name" to (info.versionName ?: ""), "code" to code.toString()))
                } catch (e: Exception) {
                    result.error("unavailable", e.message, null)
                }
            } else if (call.method == "openNotificationSettings") {
                // Push: Android stops showing the permission prompt after two
                // denials; the Device Info screen sends the operator here.
                try {
                    val intent = if (Build.VERSION.SDK_INT >= 26) {
                        Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, packageName)
                    } else {
                        Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", packageName, null))
                    }
                    startActivity(intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                    result.success(true)
                } catch (e: Exception) {
                    // Kiosk / MDM policies may block the settings app.
                    result.success(false)
                }
            } else {
                result.notImplemented()
            }
        }
    }
}
