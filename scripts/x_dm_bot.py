#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
X (Twitter) "Drop your URL" 目标站长/开发者智能私信脚本

业务背景与逻辑：
1. 在 X 上搜索 "drop your url" / "drop your project" 等推文（通常是 KOL 发起的让站长/开发者留项目链接的帖子）。
2. 在该推文的评论区中，留下 URL 的用户均为真实且极度渴望曝光、外链与流量的网站主/独立开发者。
3. 进入这些评论者的个人主页，判断其是否开放了私信（DM）权限。
4. 若开放私信，自动打开对话框，填入精心定制的介绍文案（中英双语自适应）：
   - 直击痛点：获取反向链接与流量太难；
   - 解决方案：介绍 Congrid（0质押/0手续费挂 Badge，基于语义向量匹配相似站点互相带量，赚取收益）；
   - 价值主张：说明为什么对他的项目极度有用。
5. 【安全底线】：脚本绝不自动点击发送！仅将私信填入输入框，控制台暂停等待人工复核，满意后手动点击发送，回车处理下一个。
"""

import asyncio
import json
import os
import re
import sys
import time
import urllib.parse
from typing import List, Set

from playwright.async_api import (
    Locator,
    Page,
    TimeoutError as PlaywrightTimeoutError,
    async_playwright,
)

# ==================== 配置区 ====================
CDP_URL = "http://127.0.0.1:9222"

# 新版 /i/chat 使用 textarea，旧版私信使用 contenteditable 编辑器。
# 只匹配私信编辑器及其容器，避免误填页面上的搜索框或推文输入框。
DM_COMPOSER_SELECTOR = (
    ':is([data-testid="dm-composer-textarea"], [data-testid="dmComposerTextInput"])'
    ':is(textarea, [contenteditable="true"]):visible, '
    ':is([data-testid="dm-composer-container"], [data-testid="dmComposerTextInput"]) '
    ':is(textarea, [contenteditable="true"]):visible'
)
DM_COMPOSER_TIMEOUT_MS = 15000
DM_CLOSED_INBOX_TEXT = re.compile(
    r'has a closed inbox|inbox is closed|收件箱已关闭|关闭了收件箱', re.IGNORECASE
)

# 检索目标推文的关键词
SEARCH_QUERIES: List[str] = [
    '"drop your url"',
    '"drop your project"',
    '"drop your website"',
    '"drop your link"',
]

# 单次运行最大私信准备数（建议每日控制在 5~15 人，避免触发风控）
MAX_DMS_PER_RUN = 5

# 历史已联系/已检查用户记录文件（避免重复打扰）
HISTORY_FILE = os.path.join(os.path.dirname(__file__), "seen_dm_users.json")

# ==================== 定制私信文案库 ====================
# 针对这些站长/开发者，文案定位为“独立开发者间的真诚技术与流量互助”，严禁硬推空气币

DM_TEMPLATE_EN = """Hey @{username}! 👋

Saw your project in the "drop your url" thread. As an indie builder, I know how brutal it is to get organic backlinks and real traffic nowadays without paying Google thousands in ads.

We built Congrid (https://congrid.net) — a decentralized content network on Cosmos specifically designed for site owners like us:
1. 0 Cost & 0 Staking: Simply verify domain ownership by adding a small badge to your homepage (congrid.net/publishers).
2. Traffic Cross-Pollination: The protocol uses AI semantic vector embeddings to match your site with similar content creators, exchanging verified backlinks and referral discovery.
3. Native Rewards: Sites maintaining valid verification and external links earn CONGRID token rewards, plus you can monetize ad slots (Slot/Lease) directly.

Thought it could be a great free distribution channel for your project. Keep up the awesome work! 🚀"""

DM_TEMPLATE_ZH = """哈喽 @{username}！👋

在 "drop your url" 帖子里看到了你的项目。自己做独立站深知现在的痛点：在 Google 上拿反向链接（Backlink）和自然流量太难了，全被大厂广告和 SEO 农场霸占。

我们做了一个专门帮站长破局的去中心化网络 Congrid (https://congrid.net)：
1. 零成本、免质押：在官网首页挂上验证 Badge 即可完成一级域名主权绑定（congrid.net/publishers），注册免 Gas 费；
2. 语义流量互导：协议通过向量相似度计算，将你的站点与相似优质网站自动编织为可信外链网，免费交叉带量；
3. 站长奖励与变现：保持在线与外链匹配即可每小时瓜分协议排放奖励，还能在链上出租外链位（Slot/Lease）直接变现。

觉得对你的项目出海和流量获取会很有帮助，感兴趣可以看看，祝项目越来越好！🔥"""


def load_seen_users() -> Set[str]:
    """读取历史已处理的用户名"""
    if os.path.exists(HISTORY_FILE):
        try:
            with open(HISTORY_FILE, "r", encoding="utf-8") as f:
                data = json.load(f)
                return set(data)
        except Exception:
            return set()
    return set()


def save_seen_user(username: str):
    """保存已处理的用户名"""
    users = load_seen_users()
    users.add(username.lower())
    try:
        with open(HISTORY_FILE, "w", encoding="utf-8") as f:
            json.dump(list(users), f, ensure_ascii=False, indent=2)
    except Exception as e:
        print(f"⚠️ 保存历史记录出错: {e}")


def is_chinese_text(text: str) -> bool:
    """粗略检测文本是否以中文为主"""
    chinese_chars = re.findall(r'[\u4e00-\u9fff]', text)
    return len(chinese_chars) >= 3


async def find_drop_url_threads(page: Page, query: str) -> List[str]:
    """
    在搜索结果中找到高回复量的 'drop your url' 主推文链接
    """
    print(f"\n🔍 正在检索主帖: 【{query}】...")
    encoded = urllib.parse.quote(query)
    # 搜索 Top 热门结果，回复量通常较多
    url = f"https://x.com/search?q={encoded}&f=top"

    try:
        await page.goto(url, wait_until="domcontentloaded", timeout=25000)
    except Exception as e:
        print(f"⚠️ 加载搜索页异常: {e}")

    await asyncio.sleep(4)
    await page.evaluate("window.scrollBy(0, 500)")
    await asyncio.sleep(2)

    thread_links = []
    articles = await page.locator('article[data-testid="tweet"]').all()
    print(f"📋 捕获到 {len(articles)} 条主帖候选，正在筛选...")

    for article in articles[:6]:
        try:
            # 提取推文链接
            link_el = article.locator('a[href*="/status/"]').first
            if await link_el.count() == 0:
                continue
            href = await link_el.get_attribute("href")
            if href and "/status/" in href and href not in thread_links:
                # 排除点击到引用推文的情况，只取标准推文链接
                clean_href = href.split("?")[0]
                if clean_href.count("/") == 3:  # /username/status/12345
                    thread_links.append(f"https://x.com{clean_href}")
        except Exception:
            continue

    return thread_links


async def extract_commenters_from_thread(page: Page, thread_url: str) -> List[dict]:
    """
    进入主帖，提取在评论区留链接/回复的潜在站长
    """
    print(f"\n👉 正在进入主帖评论区: {thread_url}")
    try:
        await page.goto(thread_url, wait_until="domcontentloaded", timeout=25000)
    except Exception as e:
        print(f"⚠️ 进入主帖失败: {e}")
        return []

    await asyncio.sleep(4)

    # 滚动几次以加载评论
    commenters = []
    seen_in_thread = set()

    # 提取楼主用户名，避免向楼主发（除非他也留了自己的站）
    op_username = thread_url.replace("https://x.com/", "").split("/status/")[0].lower()

    for scroll_round in range(3):
        articles = await page.locator('article[data-testid="tweet"]').all()
        # 跳过第0个（第0个通常是主贴自身）
        for article in articles[1:]:
            try:
                # 提取作者用户名链接
                user_link_el = article.locator('div[data-testid="User-Name"] a[role="link"]').first
                if await user_link_el.count() == 0:
                    continue

                user_href = await user_link_el.get_attribute("href")
                if not user_href:
                    continue

                username = user_href.strip("/").split("/")[0]
                if not username or username.lower() == op_username:
                    continue

                if username.lower() in seen_in_thread:
                    continue

                # 提取回复内容
                text_el = article.locator('[data-testid="tweetText"]').first
                reply_text = (await text_el.inner_text()).strip() if await text_el.count() > 0 else ""

                seen_in_thread.add(username.lower())
                commenters.append({
                    "username": username,
                    "reply_text": reply_text
                })
            except Exception:
                continue

        await page.evaluate("window.scrollBy(0, 800)")
        await asyncio.sleep(2)

    return commenters


class DMInboxClosedError(Exception):
    """主页存在私信入口，但对方的收件箱不接受直接私信。"""


def closed_dm_inbox_notice(page: Page) -> Locator:
    return page.locator('[data-testid="dm-composer-container"]:visible').filter(
        has_text=DM_CLOSED_INBOX_TEXT
    ).first


async def wait_for_dm_composer(
    page: Page, timeout: int = DM_COMPOSER_TIMEOUT_MS
) -> Locator:
    """等待新旧版私信窗口中的可见输入框，兼容隐藏旧节点和延迟渲染。"""
    composer = page.locator(DM_COMPOSER_SELECTOR).first
    closed_notice = closed_dm_inbox_notice(page)
    await composer.or_(closed_notice).first.wait_for(state="visible", timeout=timeout)
    if await closed_notice.is_visible():
        raise DMInboxClosedError("对方的收件箱已关闭")
    return composer


async def log_dm_composer_diagnostics(page: Page):
    """输出输入框结构供排查，不读取或记录私信正文。"""
    print(f"   页面路径: {urllib.parse.urlsplit(page.url).path}")
    candidates = await page.locator(
        'textarea, [contenteditable], [role="textbox"]'
    ).evaluate_all("""nodes => nodes.slice(0, 10).map(el => ({
        tag: el.tagName,
        testid: el.getAttribute('data-testid'),
        role: el.getAttribute('role'),
        contenteditable: el.getAttribute('contenteditable'),
        disabled: el.matches(':disabled') || el.getAttribute('aria-disabled') === 'true',
        readonly: el.hasAttribute('readonly'),
        visible: !!(el.getBoundingClientRect().width && el.getBoundingClientRect().height)
            && !['hidden', 'collapse'].includes(getComputedStyle(el).visibility)
    }))""")
    print(f"   输入框候选（最多 10 个）: {json.dumps(candidates, ensure_ascii=False)}")


async def try_prepare_dm(page: Page, user_info: dict) -> bool:
    """
    访问目标用户主页，检查 DM 权限，若开启则填写私信并等待人工复核
    """
    username = user_info["username"]
    reply_text = user_info["reply_text"]
    profile_url = f"https://x.com/{username}"

    print(f"\n--------------------------------------------------")
    print(f"👤 正在访问用户主页: @{username} ({profile_url})")
    if reply_text:
        preview = (reply_text[:80] + "...") if len(reply_text) > 80 else reply_text
        print(f"📝 他的留链留言: {preview}")

    try:
        await page.goto(profile_url, wait_until="domcontentloaded", timeout=25000)
    except Exception as e:
        print(f"⚠️ 加载个人主页超时: {e}")
        return False

    await asyncio.sleep(3)

    # 1. 检查是否存在发私信按钮 (data-testid="sendDMFromProfile")
    dm_btn = page.locator('button[data-testid="sendDMFromProfile"]').first
    has_dm_btn = await dm_btn.count() > 0

    if not has_dm_btn:
        # 尝试查找通用 aria-label 包含 Direct message 或 私信 的按钮
        dm_btn = page.locator('button[aria-label*="Direct message"], button[aria-label*="私信"], button[aria-label*="Message"]').first
        has_dm_btn = await dm_btn.count() > 0

    if not has_dm_btn:
        print(f"🚫 用户 @{username} 未开放私信权限（未开启公开发信或仅限关注者），跳过。")
        return False

    print(f"📨 已找到 @{username} 的私信入口，正在打开并检查是否可填写...")
    await dm_btn.click()

    # 2. 确定语言模板并格式化内容
    is_zh = is_chinese_text(reply_text)
    if is_zh:
        dm_content = DM_TEMPLATE_ZH.format(username=username)
        lang_label = "中文"
    else:
        dm_content = DM_TEMPLATE_EN.format(username=username)
        lang_label = "英文"

    # 3. 等待私信输入框完成渲染，再填写；fill 会等待输入框可编辑。
    try:
        composer = await wait_for_dm_composer(page, timeout=DM_COMPOSER_TIMEOUT_MS)
        await composer.fill(dm_content, timeout=DM_COMPOSER_TIMEOUT_MS)
        await asyncio.sleep(0.5)
        # 新 Chat 可能先挂载编辑器，再在权限检查结束后显示收件箱限制。
        if await closed_dm_inbox_notice(page).is_visible():
            raise DMInboxClosedError("对方的收件箱已关闭")
    except (DMInboxClosedError, PlaywrightTimeoutError) as e:
        if isinstance(e, DMInboxClosedError) or await closed_dm_inbox_notice(page).is_visible():
            print(f"🚫 用户 @{username} 的收件箱已关闭，不能通过当前入口直接私信，跳过。")
        else:
            print("⚠️ 等待可编辑的私信输入框超时，本次跳过（不写入历史记录）。")
            await log_dm_composer_diagnostics(page)
        return False

    print("\n" + "=" * 60)
    print(f"📬 【私信已成功填入输入框！】(语种: {lang_label})")
    print(f"👤 收件人: @{username}")
    print(f"💬 内容详情:\n{dm_content}")
    print("=" * 60)
    print(f"👉 安全提醒：脚本【绝对不会】自动发送！")
    print(f"   - 请切换到浏览器窗口人工审核：")
    print(f"   - 满意请【手动点击浏览器内的发送图标（Paper plane/Enter）】；")
    print(f"   - 若不合适，可直接在浏览器中清空或关闭。")
    print("=" * 60)

    # 4. 暂停等待用户手动操作
    input("👉 [请在浏览器中操作，完成后按回车键 (Enter) 寻找下一位站长] ... ")

    # 记录为已处理
    save_seen_user(username)
    return True


async def run_dm_bot():
    print("==================================================")
    print("🚀 启动 X 目标站长自动私信脚本 (Drop your URL 截流模式)")
    print(f"🔗 正在连接 Chrome CDP: {CDP_URL}")
    print("==================================================")

    seen_users = load_seen_users()
    print(f"📂 本地历史已处理用户库: {len(seen_users)} 人")

    prepared_dms_count = 0

    async with async_playwright() as p:
        try:
            browser = await p.chromium.connect_over_cdp(CDP_URL)
        except Exception as e:
            print(f"\n❌ 连接 Chrome 失败！请确保已通过 ./scripts/launch_chrome_debug.sh 启动了 Chrome。")
            print(f"报错: {e}")
            return

        contexts = browser.contexts
        if not contexts:
            print("❌ 未检测到可用浏览器上下文")
            return

        context = contexts[0]
        page = context.pages[0] if context.pages else await context.new_page()

        for query in SEARCH_QUERIES:
            if prepared_dms_count >= MAX_DMS_PER_RUN:
                break

            threads = await find_drop_url_threads(page, query)
            print(f"🎯 在关键词 【{query}】 下找到 {len(threads)} 个目标主帖")

            for thread_url in threads:
                if prepared_dms_count >= MAX_DMS_PER_RUN:
                    break

                commenters = await extract_commenters_from_thread(page, thread_url)
                print(f"👥 在主帖评论区提取到 {len(commenters)} 位潜在站长")

                for user_info in commenters:
                    if prepared_dms_count >= MAX_DMS_PER_RUN:
                        break

                    uname = user_info["username"].lower()
                    if uname in seen_users:
                        continue

                    # 尝试打开私信并预填
                    success = await try_prepare_dm(page, user_info)
                    # 本轮避免重复尝试；仅成功预填并人工处理后，才由 try_prepare_dm 持久化。
                    seen_users.add(uname)

                    if success:
                        prepared_dms_count += 1
                        print(f"✨ 今日已准备完成: {prepared_dms_count}/{MAX_DMS_PER_RUN}")
                        # 留出 3 秒给浏览器动画平复
                        await asyncio.sleep(3)

        print("\n==================================================")
        print(f"🏁 任务结束！本轮共为 {prepared_dms_count} 位目标站长准备了私信。")
        print("==================================================")


if __name__ == "__main__":
    try:
        asyncio.run(run_dm_bot())
    except KeyboardInterrupt:
        print("\n👋 脚本已由用户手动终止。")
