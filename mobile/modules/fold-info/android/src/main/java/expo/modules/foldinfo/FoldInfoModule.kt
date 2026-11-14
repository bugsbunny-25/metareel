package expo.modules.foldinfo

import android.app.Activity
import androidx.window.layout.FoldingFeature
import androidx.window.layout.WindowInfoTracker
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

private const val FOLD_CHANGE = "foldChange"

/**
 * Where the fold is, for layouts that put a split on the hinge or use the
 * tabletop posture (docs/mobile-app-plan.md, section 2). expo-hinge reports
 * the hinge's status and angle; this adds the FoldingFeature's bounds (in dp,
 * relative to the app's window), orientation, whether it separates the window
 * into two areas, and whether it occludes content. Null while the window
 * doesn't span a fold (a phone, the outer screen of a closed device).
 */
class FoldInfoModule : Module() {
  private var job: Job? = null

  @Volatile
  private var current: Map<String, Any?>? = null

  override fun definition() = ModuleDefinition {
    Name("FoldInfo")

    Events(FOLD_CHANGE)

    Function("getFold") { current }

    OnCreate { attach() }

    OnActivityEntersForeground { attach() }

    OnActivityEntersBackground {
      job?.cancel()
      job = null
    }

    OnDestroy {
      job?.cancel()
      job = null
    }
  }

  private fun attach() {
    val activity = appContext.currentActivity ?: return
    appContext.mainQueue.launch {
      if (job?.isActive == true) return@launch
      job = appContext.mainQueue.launch {
        WindowInfoTracker.getOrCreate(activity).windowLayoutInfo(activity).collect { info ->
          val fold = info.displayFeatures.filterIsInstance<FoldingFeature>().firstOrNull()
          val next = fold?.let { describe(activity, it) }
          if (next != current) {
            current = next
            sendEvent(FOLD_CHANGE, mapOf("fold" to next))
          }
        }
      }
    }
  }

  private fun describe(activity: Activity, f: FoldingFeature): Map<String, Any?> {
    val density = activity.resources.displayMetrics.density
    return mapOf(
      "left" to f.bounds.left / density,
      "top" to f.bounds.top / density,
      "width" to f.bounds.width() / density,
      "height" to f.bounds.height() / density,
      "orientation" to if (f.orientation == FoldingFeature.Orientation.VERTICAL) "vertical" else "horizontal",
      "state" to if (f.state == FoldingFeature.State.HALF_OPENED) "halfOpened" else "flat",
      "separating" to f.isSeparating,
      "occludes" to (f.occlusionType == FoldingFeature.OcclusionType.FULL)
    )
  }
}
