import { readonly, ref, type Ref } from "vue";
import * as PersonalizationService from "../../bindings/goxuetimer/personalizationservice";
import { errText } from "./format";

/**
 * 背景图的状态（模块级单例）。
 *
 * 只存"能直接放进 <img src> 的地址"：地址里的版本号是后端按文件修改时间给的，
 * 界面自己拼的话换图之后会一直显示缓存里的上一张。
 */
const imageUrl = ref("");
const picking = ref(false);
const error = ref("");

export function loadBackground(): Promise<void> {
	return PersonalizationService.Background()
		.then((state) => {
			imageUrl.value = state.url || "";
			error.value = "";
		})
		.catch((err: unknown) => {
			error.value = errText(err);
		});
}

/** 弹文件框选一张图。返回 true 表示确实换成了新图（取消为 false）。 */
export async function pickBackgroundImage(): Promise<boolean> {
	if (picking.value) return false;
	picking.value = true;
	error.value = "";
	try {
		const before = imageUrl.value;
		const state = await PersonalizationService.PickImage();
		imageUrl.value = state.url || "";
		return imageUrl.value !== "" && imageUrl.value !== before;
	} catch (err) {
		error.value = errText(err);
		return false;
	} finally {
		picking.value = false;
	}
}

export async function clearBackgroundImage(): Promise<void> {
	error.value = "";
	try {
		const state = await PersonalizationService.ClearImage();
		imageUrl.value = state.url || "";
	} catch (err) {
		error.value = errText(err);
	}
}

export function usePersonalization() {
	return {
		imageUrl: readonly(imageUrl) as Readonly<Ref<string>>,
		picking: readonly(picking) as Readonly<Ref<boolean>>,
		error: readonly(error) as Readonly<Ref<string>>,
		load: loadBackground,
		pick: pickBackgroundImage,
		clear: clearBackgroundImage,
	};
}
