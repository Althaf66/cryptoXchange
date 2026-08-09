
export const PrimaryButton = ({ children, onClick }: { children: string, onClick?: () => void }) => {
    return <button type="button" className="text-center font-semibold rounded-lg focus:ring-blue-200 focus:none focus:outline-none hover:opacity-90 disabled:opacity-80 disabled:hover:opacity-80 relative overflow-hidden h-[32px] text-sm px-3 py-1.5 mr-4 ">
        <div className="absolute inset-0 bg-blue-500 opacity-[16%]"></div>
        <div className="flex flex-row items-center justify-center gap-4"><p className="text-blue-500">{children}</p></div>
    </button>

} 

// Solid rather than the old 16%-opacity wash: at that weight, with green text on
// green, it read as a label instead of something you could press. cursor-pointer
// is deliberate too - a native <button> shows an arrow, not a hand.
export const SuccessButton = ({ children, onClick }: { children: string, onClick?: () => void }) => {
    return <button type="button" onClick={onClick} className="cursor-pointer text-center font-semibold rounded-lg bg-green-500 text-black shadow-sm focus:outline-none focus:ring-2 focus:ring-green-300 hover:bg-green-400 active:scale-95 disabled:opacity-50 h-[32px] text-sm px-4 py-1.5 mr-4 transition">
        {children}
    </button>

}