package id.co.securepatrol.tablet

import android.os.Build
import android.os.Bundle
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
            } else {
                result.notImplemented()
            }
        }
    }
}
