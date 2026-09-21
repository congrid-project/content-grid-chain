#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
X (Twitter) 智能互动与截流自动化脚本

功能优化说明：
1. 语言智能自适应：自动检测推文语种，中文推文用高情商中文回复，英文/非中文推文用地道 Native 英文回复。
2. 精准弹窗防错位：
   - 提取每篇推文独有的 status URL，杜绝 React 虚拟滚动导致的元素错位；
   - 等待弹窗内可见的回复输入框，兼容隐藏旧弹窗和异步加载；
   - 每轮交互前后强制检查并清理残留弹窗。
3. 安全机制：只将内容输入评论框，绝不自动点击发送，人工复核后回车进入下一条。
"""

import asyncio
import os
import re
import shutil
import subprocess
import sys
import time
import urllib.parse
from typing import List, Optional, Tuple
from playwright.async_api import (
    Locator,
    Page,
    TimeoutError as PlaywrightTimeoutError,
    async_playwright,
)

# ==================== 配置区 ====================
CDP_URL = "http://127.0.0.1:9222"

KEYWORDS: List[str] = [
    "vibe coding",
    "cursor traffic",
    "独立开发 流量",
    "DePIN real yield",
    "Google SEO update",
]

MAX_TWEETS_PER_KEYWORD = 5
SCHEDULE_INTERVAL_MINUTES = 0

# Antigravity CLI 命令模板 (使用 -p 无交互输出模式)
ANTIGRAVITY_CLI_CMD = ["agy", "-p", "{prompt}"]

# 不限定 div 标签，兼容不同布局中的弹窗容器。
DIALOG_SELECTOR = '[role="dialog"], [aria-modal="true"], [data-testid="sheetDialog"]'
VISIBLE_DIALOG_SELECTOR = f':is({DIALOG_SELECTOR}):visible'
REPLY_EDITOR_SELECTOR = '[contenteditable="true"]:visible'


def is_chinese_text(text: str) -> bool:
    """
    判断文本是否主要为中文
    """
    chinese_chars = re.findall(r'[\u4e00-\u9fff]', text)
    return len(chinese_chars) >= 3


def generate_reply_via_llm(tweet_text: str) -> str:
    """
    根据推文语种调用 Antigravity CLI 生成对应语言的高情绪价值回复
    """
    clean_text = tweet_text.strip().replace("\n", " ")
    is_zh = is_chinese_text(clean_text)

    if is_zh:
        prompt = (
            "请作为一位真诚、懂行的推特老友，针对以下中文推文写一条简短的互动回复：\n"
            "【核心要求】：\n"
            "1. 语言要求：必须使用纯正地道、有网感的简体中文。\n"
            "2. 态度：高度赞同推主，提供充足的“情绪价值”（真诚夸奖、赞赏敏锐观察、引发共鸣、支持打气）。\n"
            "3. 风格：像真实人类推友的随手评论，严禁AI味、翻译腔、说教或客服官腔。\n"
            "4. 字数控制：20 ~ 60 字之间，短小精悍。\n"
            "5. 输出格式：直接输出纯文本回复，严禁包含任何引号、前缀（如“回复：”）或多余说明。\n\n"
            f"目标推文内容：\n{clean_text}"
        )
    else:
        prompt = (
            "Act as an authentic, insightful tech Twitter friend and write a short, engaging reply to the following tweet:\n"
            "【Core Requirements】:\n"
            "1. Language: MUST be in natural, fluent, native-sounding conversational English.\n"
            "2. Tone: Strongly agree with the OP and provide high emotional value (warm validation, genuine praise, hype them up, acknowledge their sharp insight/analogy).\n"
            "3. Style: Sounds like a real human tech enthusiast on Twitter/X. NO robotic clichés, NO corporate jargon, NO unsolicited advice.\n"
            "4. Length: 15 to 35 words. Punchy and concise.\n"
            "5. Output format: Output ONLY the plain text reply. NO quotes, NO prefixes (e.g. 'Reply:').\n\n"
            f"Target Tweet:\n{clean_text}"
        )

    cmd = [arg.replace("{prompt}", prompt) for arg in ANTIGRAVITY_CLI_CMD]

    # 智能定位 agy 可执行文件
    binary_name = cmd[0]
    candidate_paths = [
        binary_name,
        os.path.expanduser("~/.gemini/antigravity/bin/agy"),
        "/usr/local/bin/agy",
        "/opt/homebrew/bin/agy",
        os.path.expanduser("~/.local/bin/agy"),
    ]
    for p in candidate_paths:
        if shutil.which(p) or (os.path.exists(p) and os.access(p, os.X_OK)):
            cmd[0] = p
            break

    try:
        result = subprocess.run(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=30,
        )
        if result.returncode == 0 and result.stdout.strip():
            reply = result.stdout.strip()
            reply = reply.strip('"\'').strip()
            return reply
        else:
            err = result.stderr.strip()
            print(f"⚠️ [CLI 提示] 命令未返回有效结果，使用高情商备选模板。")
    except FileNotFoundError:
        print(f"⚠️ [CLI 提示] 未在系统中找到命令 '{cmd[0]}'，使用高情商备用模板。")
    except Exception as e:
        print(f"⚠️ [CLI 异常] 调用发生错误: {e}")

    # 语种对应的备选模板
    import random
    if is_zh:
        fallback_zh = [
            "太赞同了，说到心坎里了！这年头能把这个痛点看得这么透彻的人真不多，给你点个大大的赞！🔥",
            "狠狠共鸣了！每一句都在点子上，尤其是真实感受太有说服力了，必须支持一波！👏",
            "说得太真实了！现在的环境确实需要更多这种清醒的声音，感谢分享，持续关注！✨",
        ]
        return random.choice(fallback_zh)
    else:
        fallback_en = [
            "Spot on! Couldn't agree more, this analogy hits the nail right on the head. 🔥",
            "Such an insightful take. The framing here is so refreshing and completely true! 👏",
            "100% this. Really well articulated, love seeing clear-headed perspectives like this on the timeline! ✨",
        ]
        return random.choice(fallback_en)


async def wait_for_reply_editor(page: Page, timeout: int = 15000) -> Locator:
    """等待任一弹窗内的可见回复编辑器，避免被隐藏的旧弹窗阻塞。"""
    editor = page.locator(DIALOG_SELECTOR).locator(REPLY_EDITOR_SELECTOR).first
    # 等待输入框本身：弹窗容器出现时，React 可能尚未挂载编辑器。
    await editor.wait_for(state="visible", timeout=timeout)
    return editor


async def ensure_dialog_closed(page: Page):
    """
    强制清理屏幕上残留的弹窗，确保主信息流可见可点
    """
    for _ in range(3):
        dialogs = page.locator(VISIBLE_DIALOG_SELECTOR)
        if await dialogs.count() == 0:
            break
        close_btn = dialogs.last.locator('[data-testid="app-bar-close"]:visible').first
        if await close_btn.count() > 0:
            try:
                await close_btn.click()
            except Exception:
                await page.keyboard.press("Escape")
        else:
            await page.keyboard.press("Escape")
        await asyncio.sleep(0.8)


async def process_keyword(page: Page, keyword: str):
    """
    精准遍历处理当前关键词下的推文
    """
    print(f"\n==========================================")
    print(f"🔍 正在检索关键词: 【{keyword}】")
    print(f"==========================================")

    # 启动前清理任何可能遗留的弹窗
    await ensure_dialog_closed(page)

    encoded_kw = urllib.parse.quote(keyword)
    target_url = f"https://x.com/search?q={encoded_kw}&f=live"

    try:
        await page.goto(target_url, wait_until="domcontentloaded", timeout=25000)
    except Exception as e:
        print(f"⚠️ 页面加载超时或失败: {e}")

    await asyncio.sleep(3)

    seen_status_urls = set()
    processed_count = 0
    consecutive_no_new = 0

    while processed_count < MAX_TWEETS_PER_KEYWORD and consecutive_no_new < 4:
        # 获取当前页面可视范围内的 articles
        articles = await page.locator('article[data-testid="tweet"]').all()
        found_in_batch = False

        for article in articles:
            if processed_count >= MAX_TWEETS_PER_KEYWORD:
                break

            try:
                # 1. 获取推文专属 URL 作为唯一键（例如 /username/status/123456789）
                status_link = article.locator('a[href*="/status/"]').first
                if await status_link.count() == 0:
                    continue

                tweet_href = await status_link.get_attribute("href")
                if not tweet_href or tweet_href in seen_status_urls:
                    continue

                # 2. 过滤广告推文
                is_promoted = (
                    await article.locator('text="Promoted"').count() > 0 or
                    await article.locator('text="赞助"').count() > 0
                )
                if is_promoted:
                    seen_status_urls.add(tweet_href)
                    continue

                # 3. 提取推文正文
                text_el = article.locator('[data-testid="tweetText"]').first
                if await text_el.count() == 0:
                    continue

                tweet_text = (await text_el.inner_text()).strip()
                if not tweet_text:
                    continue

                # 记录为已处理
                seen_status_urls.add(tweet_href)
                found_in_batch = True
                processed_count += 1

                lang_tag = "中文" if is_chinese_text(tweet_text) else "英文/外文"
                print(f"\n----------------------------------------")
                print(f"📌 [推文 {processed_count}/{MAX_TWEETS_PER_KEYWORD}] (语种: {lang_tag})")
                print(f"🔗 链接: https://x.com{tweet_href}")
                preview = (tweet_text[:120] + "...") if len(tweet_text) > 120 else tweet_text
                print(f"内容摘要: {preview}")

                # 4. 生成对应语种的回复
                print(f"🤖 正在调用 Antigravity CLI 生成【{lang_tag}】高情绪价值回复...")
                reply_content = generate_reply_via_llm(tweet_text)
                print(f"💡 [生成建议]: {reply_content}")

                # 5. 确保此时没有旧弹窗挡路
                await ensure_dialog_closed(page)

                # 6. 将推文滚动到视窗中央并点击当前推文的 Reply 按钮
                await article.scroll_into_view_if_needed()
                await asyncio.sleep(0.8)

                reply_btn = article.locator('button[data-testid="reply"]').first
                if await reply_btn.count() == 0:
                    print("⚠️ 当前推文未找到回复按钮，跳过")
                    continue

                await reply_btn.click()

                # 7. 等待弹窗内实际可见的输入框，不固定选择第一个弹窗或 _0 编辑器。
                try:
                    textarea = await wait_for_reply_editor(page)
                except PlaywrightTimeoutError as e:
                    visible_dialog_count = await page.locator(VISIBLE_DIALOG_SELECTOR).count()
                    if visible_dialog_count:
                        print("⚠️ 回复弹窗已出现，但等待可见回复输入框超时，跳过本条。")
                    else:
                        print("⚠️ 等待回复输入框超时，未检测到可见弹窗，跳过本条。")
                    print(f"   定位详情: {e}")
                    await ensure_dialog_closed(page)
                    continue

                await textarea.click()
                await asyncio.sleep(0.3)
                await textarea.fill(reply_content)
                await asyncio.sleep(0.5)

                print("\n" + "=" * 55)
                print(f"✅ 【内容已准确填入对应推文的输入框！】")
                print(f"💬 内容: {reply_content}")
                print(f"👉 安全提醒：脚本【不会】自动发送！请在浏览器中人工复核。")
                print(f"   - 满意请在浏览器中手动点击【Reply/回复】发送；")
                print(f"   - 如不想发送可直接关掉弹窗或按回车跳过。")
                print("=" * 55)

                input("👉 [请在浏览器中查看，按回车键 (Enter) 继续下一条推文] ... ")

                # 8. 人工复核后，若弹窗仍未关闭（例如用户取消发送），则清理关闭弹窗
                await ensure_dialog_closed(page)
                await asyncio.sleep(1)

            except Exception as e:
                print(f"⚠️ 处理单条推文出错: {e}")
                await ensure_dialog_closed(page)
                continue

        # 向下滚动加载更多推文
        await page.evaluate("window.scrollBy(0, 700)")
        await asyncio.sleep(2.5)

        if not found_in_batch:
            consecutive_no_new += 1
        else:
            consecutive_no_new = 0

    print(f"\n✨ 关键词 【{keyword}】 互动巡视完毕，共处理 {processed_count} 篇推文。")


async def run_bot():
    print("==================================================")
    print("🚀 启动 X 自动化互动脚本 (精准防错位 + 中英文自适应版)")
    print(f"🔗 正在尝试连接 Chrome CDP: {CDP_URL}")
    print("==================================================")

    async with async_playwright() as p:
        try:
            browser = await p.chromium.connect_over_cdp(CDP_URL)
        except Exception as e:
            print(f"\n❌ 连接 Chrome 失败！请确保已通过 launch_chrome_debug.sh 启动了 Chrome。")
            print(f"报错信息: {e}")
            return

        contexts = browser.contexts
        if not contexts:
            print("❌ 未找到可用的浏览器上下文")
            return

        context = contexts[0]
        page = context.pages[0] if context.pages else await context.new_page()

        while True:
            for kw in KEYWORDS:
                await process_keyword(page, kw)
                await asyncio.sleep(3)

            if SCHEDULE_INTERVAL_MINUTES <= 0:
                print("\n🏁 一次性任务全部完成，脚本正常退出。")
                break

            print(f"\n⏳ 等待 {SCHEDULE_INTERVAL_MINUTES} 分钟后进行下一轮巡视...")
            await asyncio.sleep(SCHEDULE_INTERVAL_MINUTES * 60)


if __name__ == "__main__":
    try:
        asyncio.run(run_bot())
    except KeyboardInterrupt:
        print("\n👋 脚本已由用户手动终止。")
